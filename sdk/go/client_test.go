package nervsdk_test

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
	nervsdk "hop.top/nerv/sdk/go"
)

// fakeEventService is an in-test ConnectRPC handler used by the SDK
// tests to avoid coupling sdk/go to internal/daemon. It echoes a fixed
// decision/list so the assertions stay deterministic.
type fakeEventService struct {
	dispatches int
	lists      int
}

func (f *fakeEventService) Dispatch(
	_ context.Context, req *connect.Request[nervv1.DispatchRequest],
) (*connect.Response[nervv1.DispatchResponse], error) {
	f.dispatches++
	msg := "ok"
	if env := req.Msg.GetEnvelope(); env != nil && env.GetEvent() != "" {
		msg = "received:" + env.GetEvent()
	}
	return connect.NewResponse(&nervv1.DispatchResponse{
		Decision: &nervv1.Decision{
			Action:  nervv1.Action_ACTION_ALLOW,
			Message: msg,
		},
	}), nil
}

func (f *fakeEventService) List(
	context.Context, *connect.Request[nervv1.ListRequest],
) (*connect.Response[nervv1.ListResponse], error) {
	f.lists++
	return connect.NewResponse(&nervv1.ListResponse{
		Events: []*nervv1.EventDescriptor{
			{Name: "PreToolUse", Category: "tool", Origin: "native", Blocking: true},
			{Name: "SessionStart", Category: "session", Origin: "native"},
		},
	}), nil
}

// startFakeDaemon spins up an in-process ConnectRPC EventService on a
// UDS and returns the socket path plus a cleanup func. Mirrors the
// real daemon transport (h2c + UDS) so the SDK test exercises the
// full wire path.
func startFakeDaemon(t *testing.T, svc nervv1connect.EventServiceHandler) (string, func()) {
	t.Helper()

	// macOS sun_path is 104 bytes; t.TempDir() embeds the test name
	// and can overflow. os.MkdirTemp keeps the path short.
	dir, err := os.MkdirTemp("", "nervsdk-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	sock := dir + "/n.sock"

	mux := http.NewServeMux()
	path, handler := nervv1connect.NewEventServiceHandler(svc)
	mux.Handle(path, handler)

	ln, err := net.Listen("unix", sock)
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("Listen unix: %v", err)
	}

	srv := &http.Server{
		Handler:           mux,
		Protocols:         daemonProtocols(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		_ = srv.Serve(ln)
	}()

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = ln.Close()
		_ = os.RemoveAll(dir)
	}

	if err := waitForSocket(sock); err != nil {
		cleanup()
		t.Fatalf("waitForSocket: %v", err)
	}
	return sock, cleanup
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

func TestClient_Dispatch_OverUDS(t *testing.T) {
	t.Parallel()

	svc := &fakeEventService{}
	sock, cleanup := startFakeDaemon(t, svc)
	defer cleanup()

	client := nervsdk.NewUDS(sock)
	defer func() { _ = client.Close() }() // teardown; Close itself is tested below

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	decision, err := client.Dispatch(ctx, &nervv1.Envelope{
		NervVersion: "1.0.0-draft",
		Event:       "PreToolUse",
		EventId:     "00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if decision == nil {
		t.Fatal("Dispatch: nil decision")
	}
	if got, want := decision.GetMessage(), "received:PreToolUse"; got != want {
		t.Errorf("decision.Message = %q, want %q", got, want)
	}
	if got, want := decision.GetAction(), nervv1.Action_ACTION_ALLOW; got != want {
		t.Errorf("decision.Action = %v, want %v", got, want)
	}
	if svc.dispatches != 1 {
		t.Errorf("dispatches = %d, want 1", svc.dispatches)
	}
}

func TestClient_List_OverUDS(t *testing.T) {
	t.Parallel()

	svc := &fakeEventService{}
	sock, cleanup := startFakeDaemon(t, svc)
	defer cleanup()

	client := nervsdk.NewUDS(sock)
	defer func() { _ = client.Close() }() // teardown; Close itself is tested below

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := client.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].GetName() != "PreToolUse" {
		t.Errorf("events[0].Name = %q, want %q", events[0].GetName(), "PreToolUse")
	}
}

func TestClient_Close_Idempotent(t *testing.T) {
	t.Parallel()

	client := nervsdk.NewUDS("/nonexistent.sock")
	if err := client.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestDefaultSocketPath_RespectsEnv(t *testing.T) {
	// Not t.Parallel — mutates env.
	t.Setenv(nervsdk.EnvSocket, "/custom/path.sock")
	if got, want := nervsdk.DefaultSocketPath(), "/custom/path.sock"; got != want {
		t.Errorf("DefaultSocketPath = %q, want %q", got, want)
	}
}

func TestDefaultSocketPath_XDGFallback(t *testing.T) {
	// Not t.Parallel — mutates env.
	t.Setenv(nervsdk.EnvSocket, "")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got, want := nervsdk.DefaultSocketPath(), "/run/user/1000/nerv.sock"; got != want {
		t.Errorf("DefaultSocketPath = %q, want %q", got, want)
	}
}

// daemonProtocols mirrors the daemon: HTTP/1.1 plus cleartext HTTP/2.
func daemonProtocols() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return p
}
