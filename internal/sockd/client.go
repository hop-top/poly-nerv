//go:build !windows

package sockd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

// DefaultSocketPath returns the socket path using XDG conventions,
// matching the daemon's path resolution.
func DefaultSocketPath() string {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	return filepath.Join(runtimeDir, "nerv.sock")
}

// SocketAvailable checks whether the daemon socket exists and
// accepts connections. Returns the socket path if available.
func SocketAvailable() (string, bool) {
	sock := os.Getenv("NERV_SOCKET")
	if sock == "" {
		sock = DefaultSocketPath()
	}
	if _, err := os.Stat(sock); err != nil {
		return sock, false
	}
	conn, err := net.DialTimeout("unix", sock, 500*time.Millisecond)
	if err != nil {
		return sock, false
	}
	_ = conn.Close()
	return sock, true
}

// Client connects to the daemon socket and sends envelopes.
type Client struct {
	conn    net.Conn
	sock    string
	cliName string
	cliVer  string
	sessID  string
}

// Dial connects to the daemon socket and completes the handshake.
func Dial(sock, cliName, cliVersion, sessionID string) (*Client, error) {
	conn, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("sockd: dial %s: %w", sock, err)
	}

	c := &Client{
		conn:    conn,
		sock:    sock,
		cliName: cliName,
		cliVer:  cliVersion,
		sessID:  sessionID,
	}

	if err := c.handshake(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return c, nil
}

func (c *Client) handshake() error {
	hello := msg{
		Type:            "hello",
		ProtocolVersion: protocolVersion,
		SessionID:       c.sessID,
		CLI: mustMarshal(map[string]string{
			"name":    c.cliName,
			"version": c.cliVer,
		}),
	}
	helloBytes, _ := json.Marshal(hello)

	_ = c.conn.SetDeadline(time.Now().Add(handshakeTimeout))
	if err := writeMsg(c.conn, helloBytes); err != nil {
		return fmt.Errorf("sockd: write hello: %w", err)
	}

	ackBytes, err := readMsg(c.conn)
	if err != nil {
		return fmt.Errorf("sockd: read hello_ack: %w", err)
	}

	var ack msg
	if err := json.Unmarshal(ackBytes, &ack); err != nil {
		return fmt.Errorf("sockd: unmarshal hello_ack: %w", err)
	}
	if ack.Type == "error" {
		return fmt.Errorf("sockd: server error: %s: %s", ack.Code, ack.Message)
	}
	if ack.Type != "hello_ack" {
		return fmt.Errorf("sockd: expected hello_ack, got %q", ack.Type)
	}

	_ = c.conn.SetDeadline(time.Time{})
	return nil
}

// Send sends an envelope and returns the decision response.
func (c *Client) Send(id string, envelope json.RawMessage) (json.RawMessage, error) {
	m := msg{
		Type:     "envelope",
		ID:       id,
		Envelope: envelope,
	}
	mBytes, _ := json.Marshal(m)

	_ = c.conn.SetDeadline(time.Now().Add(30 * time.Second))
	if err := writeMsg(c.conn, mBytes); err != nil {
		return nil, fmt.Errorf("sockd: write envelope: %w", err)
	}

	respBytes, err := readMsg(c.conn)
	if err != nil {
		return nil, fmt.Errorf("sockd: read decision: %w", err)
	}

	var resp msg
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("sockd: unmarshal response: %w", err)
	}
	if resp.Type == "error" {
		return nil, fmt.Errorf("sockd: server error: %s: %s", resp.Code, resp.Message)
	}
	if resp.Type != "decision" {
		return nil, fmt.Errorf("sockd: expected decision, got %q", resp.Type)
	}

	return resp.Decision, nil
}

// Close sends a goodbye message and closes the connection.
func (c *Client) Close() error {
	bye := msg{Type: "goodbye", Reason: "session_end"}
	byeBytes, _ := json.Marshal(bye)
	_ = writeMsg(c.conn, byeBytes)
	return c.conn.Close()
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
