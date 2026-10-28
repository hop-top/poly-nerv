//go:build !windows

// Package sockd implements the nerv daemon socket server per
// spec/socket-protocol.md. It listens on a Unix domain socket,
// accepts connections, performs the hello/hello_ack handshake,
// then processes envelope→decision pairs using the dispatch
// pipeline.
package sockd

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

// maxMessageSize per spec: 10 MiB.
const maxMessageSize = 10 << 20

// protocolVersion is the current protocol version.
const protocolVersion = "1.0"

// Limits per spec.
const (
	maxConns          = 64
	maxInflight       = 16
	handshakeTimeout  = 5 * time.Second
	heartbeatInterval = 30 * time.Second
	heartbeatMissTol  = 3
)

// Handler processes an envelope JSON and returns a decision JSON.
type Handler func(envelopeJSON json.RawMessage) (json.RawMessage, error)

// Server is the nerv daemon socket server.
type Server struct {
	sockPath string
	handler  Handler
	version  string
	listener net.Listener

	mu      sync.Mutex
	conns   map[net.Conn]struct{}
	connSem chan struct{} // bounds concurrent connections
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	logger  *log.Logger
}

// New creates a server that will listen on sockPath.
func New(sockPath string, handler Handler, version string) *Server {
	return &Server{
		sockPath: sockPath,
		handler:  handler,
		version:  version,
		conns:    make(map[net.Conn]struct{}),
		connSem:  make(chan struct{}, maxConns),
		logger:   log.New(os.Stderr, "sockd: ", log.LstdFlags),
	}
}

// ListenAndServe starts listening on the Unix socket. Blocks until
// ctx is cancelled or an unrecoverable error occurs.
func (s *Server) ListenAndServe(ctx context.Context) error {
	// Remove stale socket file.
	_ = os.Remove(s.sockPath)

	// Ensure parent directory exists.
	if err := os.MkdirAll(parentDir(s.sockPath), 0o700); err != nil {
		return fmt.Errorf("sockd: mkdir: %w", err)
	}

	ln, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return fmt.Errorf("sockd: listen: %w", err)
	}
	s.listener = ln

	// Set socket permissions to 0600 (owner-only).
	if err := os.Chmod(s.sockPath, 0o600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("sockd: chmod: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.logger.Printf("listening on %s", s.sockPath)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break // shutting down
			}
			if errors.Is(err, net.ErrClosed) {
				break
			}
			s.logger.Printf("accept error: %v", err)
			continue
		}

		// Enforce connection limit.
		select {
		case s.connSem <- struct{}{}:
		default:
			s.sendError(conn, "OVERLOADED", "max connections reached")
			_ = conn.Close()
			continue
		}

		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() {
				<-s.connSem
				s.mu.Lock()
				delete(s.conns, conn)
				s.mu.Unlock()
				_ = conn.Close()
			}()
			s.handleConn(ctx, conn)
		}()
	}

	s.wg.Wait()
	_ = os.Remove(s.sockPath)
	return nil
}

// Shutdown initiates graceful shutdown: sends shutdown message to
// all connected clients, waits for in-flight work, then closes.
func (s *Server) Shutdown() {
	shutdownMsg := msg{Type: "shutdown", Reason: "daemon_restart"}
	raw, _ := json.Marshal(shutdownMsg)

	s.mu.Lock()
	for c := range s.conns {
		_ = writeMsg(c, raw)
	}
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
}

// handleConn manages a single client connection lifecycle.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	// Handshake phase.
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	hello, err := readMsg(conn)
	if err != nil {
		s.logger.Printf("handshake read: %v", err)
		s.sendError(conn, "HANDSHAKE_TIMEOUT", "failed to read hello")
		return
	}

	var helloMsg msg
	if err := json.Unmarshal(hello, &helloMsg); err != nil {
		s.sendError(conn, "PARSE_ERROR", "invalid hello JSON")
		return
	}
	if helloMsg.Type != "hello" {
		s.sendError(conn, "UNKNOWN_TYPE", "expected hello message")
		return
	}
	if helloMsg.ProtocolVersion != "" && helloMsg.ProtocolVersion != protocolVersion {
		s.sendError(conn, "VERSION_MISMATCH",
			fmt.Sprintf("server protocol %s, client %s", protocolVersion, helloMsg.ProtocolVersion))
		return
	}

	// Send hello_ack.
	ack := msg{
		Type:        "hello_ack",
		NervVersion: s.version,
		Capabilities: []string{
			"redaction",
			"decision_merge",
			"event_log",
		},
	}
	ackBytes, _ := json.Marshal(ack)
	if err := writeMsg(conn, ackBytes); err != nil {
		s.logger.Printf("hello_ack write: %v", err)
		return
	}

	// Clear deadline for steady-state.
	_ = conn.SetDeadline(time.Time{})

	// Steady-state: read messages, dispatch envelopes.
	for {
		if ctx.Err() != nil {
			return
		}

		// Set read deadline for heartbeat detection.
		_ = conn.SetReadDeadline(
			time.Now().Add(heartbeatInterval * time.Duration(heartbeatMissTol+1)),
		)

		raw, err := readMsg(conn)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			s.logger.Printf("read: %v", err)
			return
		}

		var incoming msg
		if err := json.Unmarshal(raw, &incoming); err != nil {
			s.sendError(conn, "PARSE_ERROR", "invalid JSON")
			return
		}

		switch incoming.Type {
		case "envelope":
			resp, err := s.dispatchEnvelope(incoming)
			if err != nil {
				s.logger.Printf("dispatch error: %v", err)
				errMsg := msg{Type: "error", Code: "INTERNAL", Message: err.Error()}
				errBytes, _ := json.Marshal(errMsg)
				_ = writeMsg(conn, errBytes)
				continue
			}
			if err := writeMsg(conn, resp); err != nil {
				s.logger.Printf("write decision: %v", err)
				return
			}

		case "heartbeat":
			// No response needed per spec.

		case "goodbye":
			return

		default:
			s.sendError(conn, "UNKNOWN_TYPE", fmt.Sprintf("unknown type %q", incoming.Type))
			return
		}
	}
}

// dispatchEnvelope processes a single envelope message.
func (s *Server) dispatchEnvelope(m msg) ([]byte, error) {
	decisionJSON, err := s.handler(m.Envelope)
	if err != nil {
		return nil, err
	}

	resp := msg{
		Type:     "decision",
		ID:       m.ID,
		Decision: decisionJSON,
	}
	return json.Marshal(resp)
}

func (s *Server) sendError(conn net.Conn, code, message string) {
	e := msg{Type: "error", Code: code, Message: message}
	raw, _ := json.Marshal(e)
	_ = writeMsg(conn, raw)
}

// msg is the generic wire message supporting all types.
type msg struct {
	Type            string          `json:"type"`
	ID              string          `json:"id,omitempty"`
	ProtocolVersion string          `json:"protocol_version,omitempty"`
	NervVersion     string          `json:"nerv_version,omitempty"`
	Capabilities    []string        `json:"capabilities,omitempty"`
	Envelope        json.RawMessage `json:"envelope,omitempty"`
	Decision        json.RawMessage `json:"decision,omitempty"`
	Code            string          `json:"code,omitempty"`
	Message         string          `json:"message,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	CLI             json.RawMessage `json:"cli,omitempty"`
	SessionID       string          `json:"session_id,omitempty"`
	Ts              string          `json:"ts,omitempty"`
}

// readMsg reads a length-prefixed message from the connection.
func readMsg(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, err
	}
	if length > maxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// writeMsg writes a length-prefixed message to the connection.
func writeMsg(w io.Writer, data []byte) error {
	if uint32(len(data)) > maxMessageSize {
		return fmt.Errorf("message too large: %d bytes", len(data))
	}
	if err := binary.Write(w, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err := w.Write(data)
	return err
}

// parentDir returns the parent directory of a path.
func parentDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
