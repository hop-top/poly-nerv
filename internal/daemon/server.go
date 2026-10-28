// Package daemon hosts the ConnectRPC EventService over a Unix domain
// socket, serving HTTP/1.1 and cleartext HTTP/2 (h2c, prior knowledge)
// via net/http's Protocols. It replaces the legacy stdin/stdout NDJSON dispatcher in
// cmd/nervd and is wired into a "nerv daemon" subcommand in a later task.
//
// The Service interface deliberately uses plain proto types (not
// connect.Request[T] wrappers) so business logic can be unit-tested
// without ConnectRPC machinery. A thin connectEventService adapter
// bridges Service to nervv1connect.EventServiceHandler.
package daemon

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"

	nervv1 "hop.top/nerv/gen/go/nerv/v1"
	"hop.top/nerv/gen/go/nerv/v1/nervv1connect"
)

// Config holds daemon construction parameters.
type Config struct {
	// SocketPath is the Unix domain socket to listen on (e.g.
	// /run/nerv.sock). Required.
	SocketPath string
	// Service implements the EventService RPC methods.
	Service EventService
}

// EventService is the contract the daemon needs from its event-dispatch
// backend. Plain proto types keep tests free of connect.Request wrappers.
// Real wiring (nerv hot-path dispatcher) lands in a later task; until
// then a stub returning placeholder responses is sufficient.
type EventService interface {
	Dispatch(context.Context, *nervv1.DispatchRequest) (*nervv1.DispatchResponse, error)
	List(context.Context, *nervv1.ListRequest) (*nervv1.ListResponse, error)
}

// Server is the daemon HTTP server hosting ConnectRPC handlers.
type Server struct {
	cfg Config
	mux *http.ServeMux
}

// New constructs a Server. It returns an error if the config is invalid.
func New(cfg Config) (*Server, error) {
	if cfg.SocketPath == "" {
		return nil, errors.New("daemon: SocketPath required")
	}
	if cfg.Service == nil {
		return nil, errors.New("daemon: Service required")
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.registerHandlers()
	return s, nil
}

// Serve listens on the configured UDS until ctx is canceled.
func (s *Server) Serve(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.cfg.SocketPath), 0o755); err != nil {
		return err
	}
	_ = os.Remove(s.cfg.SocketPath) // ignore "not exist"
	ln, err := net.Listen("unix", s.cfg.SocketPath)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }() // usually already closed by srv; a second close is harmless

	srv := &http.Server{
		Handler:   s.mux,
		Protocols: protocols(),
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		_ = srv.Close()
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) registerHandlers() {
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	path, handler := nervv1connect.NewEventServiceHandler(
		&connectEventService{inner: s.cfg.Service},
	)
	s.mux.Handle(path, handler)
}

// protocols enables HTTP/1.1 (Connect over HTTP/1.1, the Go and TS
// SDKs) and cleartext HTTP/2 with prior knowledge (gRPC clients: the
// Python, Rust and PHP SDKs).
func protocols() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return p
}
