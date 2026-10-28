package daemon

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"

	nervv1 "hop.top/nerv/gen/go/nerv/v1"
	"hop.top/nerv/gen/go/nerv/v1/nervv1connect"
)

// TestNewServerListensOnUDS spawns a Server on a temp socket, waits for
// the socket to appear, cancels the context, and asserts a clean exit.
// This is the canonical "is the daemon alive" smoke test.
func TestNewServerListensOnUDS(t *testing.T) {
	t.Parallel()

	sock := tempSocket(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := New(Config{
		SocketPath: sock,
		Service:    &fakeEventService{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()

	if err := waitForSocket(sock); err != nil {
		select {
		case serveErr := <-errCh:
			t.Fatalf("waitForSocket: %v; Serve error: %v", err, serveErr)
		default:
			t.Fatalf("waitForSocket: %v", err)
		}
	}

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve returned: %v", err)
	}
}

// TestServer_Dispatch_OverUDS round-trips a Dispatch call from a
// connectrpc client through h2c-over-UDS into the Server. Asserts the
// transport wiring is correct end-to-end; placeholder responses are
// fine, the real dispatcher lands later.
func TestServer_Dispatch_OverUDS(t *testing.T) {
	t.Parallel()

	sock := tempSocket(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := New(Config{
		SocketPath: sock,
		Service:    &fakeEventService{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-errCh
	})

	if err := waitForSocket(sock); err != nil {
		select {
		case serveErr := <-errCh:
			t.Fatalf("waitForSocket: %v; Serve error: %v", err, serveErr)
		default:
			t.Fatalf("waitForSocket: %v", err)
		}
	}

	client := nervv1connect.NewEventServiceClient(
		&http.Client{Transport: udsTransport(sock)},
		"http://unix",
	)

	resp, err := client.Dispatch(ctx, connect.NewRequest(&nervv1.DispatchRequest{}))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if resp == nil || resp.Msg == nil {
		t.Fatalf("Dispatch: nil response")
	}
}

// fakeEventService is a placeholder Service for transport tests.
type fakeEventService struct{}

func (f *fakeEventService) Dispatch(context.Context, *nervv1.DispatchRequest) (*nervv1.DispatchResponse, error) {
	return &nervv1.DispatchResponse{}, nil
}

func (f *fakeEventService) List(context.Context, *nervv1.ListRequest) (*nervv1.ListResponse, error) {
	return &nervv1.ListResponse{}, nil
}

// tempSocket returns a short UDS path that fits in the 104-byte sun_path
// cap on darwin. t.TempDir() embeds the test name and can overflow.
func tempSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nerv-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir + "/n.sock"
}

func waitForSocket(path string) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("socket did not appear")
}

// udsTransport returns an http.RoundTripper that dials the given UDS
// path. ConnectRPC speaks Connect protocol over HTTP/1.1 by default so
// no h2c-specific transport is needed on the client side; the server
// also accepts cleartext HTTP/2 (http.Protocols) for gRPC/h2c clients.
func udsTransport(sock string) http.RoundTripper {
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
	}
}
