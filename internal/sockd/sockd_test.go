//go:build !windows

package sockd

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// echoHandler returns the envelope as-is wrapped in a decision.
func echoHandler(envJSON json.RawMessage) (json.RawMessage, error) {
	d := map[string]any{
		"action":   "allow",
		"warnings": []string{},
	}
	return json.Marshal(d)
}

func writeTestMsg(conn net.Conn, data []byte) error {
	return writeMsg(conn, data)
}

func readTestMsg(conn net.Conn) ([]byte, error) {
	return readMsg(conn)
}

func doHandshake(t *testing.T, conn net.Conn) {
	t.Helper()
	hello := msg{
		Type:            "hello",
		ProtocolVersion: "1.0",
		SessionID:       "test-session",
	}
	helloBytes, _ := json.Marshal(hello)
	if err := writeTestMsg(conn, helloBytes); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	ackBytes, err := readTestMsg(conn)
	if err != nil {
		t.Fatalf("read hello_ack: %v", err)
	}
	var ack msg
	if err := json.Unmarshal(ackBytes, &ack); err != nil {
		t.Fatalf("unmarshal hello_ack: %v", err)
	}
	if ack.Type != "hello_ack" {
		t.Fatalf("expected hello_ack, got %q", ack.Type)
	}
}

// shortTempDir returns a short temp dir for unix sockets (macOS
// has a 104-byte path limit for AF_UNIX).
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "nerv-")
	if err != nil {
		t.Fatalf("mktmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func startTestServer(t *testing.T) (string, context.CancelFunc) {
	t.Helper()
	sock := filepath.Join(shortTempDir(t), "t.sock")
	srv := New(sock, echoHandler, "test")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.ListenAndServe(ctx)
	}()

	// Wait for socket to appear.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("server did not stop in time")
		}
	})

	return sock, cancel
}

func TestHandshake(t *testing.T) {
	sock, _ := startTestServer(t)

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	doHandshake(t, conn)
}

func TestEnvelopeRoundtrip(t *testing.T) {
	sock, _ := startTestServer(t)

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	doHandshake(t, conn)

	// Send an envelope.
	envPayload := json.RawMessage(`{
		"event": "PreToolUse",
		"cli": {"name": "claude", "version": "1.0"},
		"session": {"id": "sess_1"},
		"timestamp": "2026-04-11T12:00:00Z"
	}`)
	envMsg := msg{
		Type:     "envelope",
		ID:       "evt_001",
		Envelope: envPayload,
	}
	envBytes, _ := json.Marshal(envMsg)
	if err := writeTestMsg(conn, envBytes); err != nil {
		t.Fatalf("write envelope: %v", err)
	}

	// Read decision.
	respBytes, err := readTestMsg(conn)
	if err != nil {
		t.Fatalf("read decision: %v", err)
	}
	var resp msg
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		t.Fatalf("unmarshal decision: %v", err)
	}
	if resp.Type != "decision" {
		t.Errorf("expected decision, got %q", resp.Type)
	}
	if resp.ID != "evt_001" {
		t.Errorf("expected id evt_001, got %q", resp.ID)
	}

	// Verify decision payload.
	var d map[string]any
	if err := json.Unmarshal(resp.Decision, &d); err != nil {
		t.Fatalf("unmarshal decision payload: %v", err)
	}
	if d["action"] != "allow" {
		t.Errorf("expected allow, got %v", d["action"])
	}
}

func TestGoodbye(t *testing.T) {
	sock, _ := startTestServer(t)

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	doHandshake(t, conn)

	// Send goodbye.
	bye := msg{Type: "goodbye", Reason: "session_end"}
	byeBytes, _ := json.Marshal(bye)
	if err := writeTestMsg(conn, byeBytes); err != nil {
		t.Fatalf("write goodbye: %v", err)
	}

	// Connection should be closed by server.
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = readTestMsg(conn)
	if err == nil {
		t.Error("expected read error after goodbye, got nil")
	}
}

func TestUnknownTypeClosesConnection(t *testing.T) {
	sock, _ := startTestServer(t)

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	doHandshake(t, conn)

	// Send unknown type.
	unknown := msg{Type: "foobar"}
	ub, _ := json.Marshal(unknown)
	if err := writeTestMsg(conn, ub); err != nil {
		t.Fatalf("write unknown: %v", err)
	}

	// Should get error then close.
	respBytes, err := readTestMsg(conn)
	if err != nil {
		t.Fatalf("read error msg: %v", err)
	}
	var resp msg
	_ = json.Unmarshal(respBytes, &resp)
	if resp.Type != "error" {
		t.Errorf("expected error, got %q", resp.Type)
	}
	if resp.Code != "UNKNOWN_TYPE" {
		t.Errorf("expected UNKNOWN_TYPE, got %q", resp.Code)
	}
}

func TestNoHelloTimesOut(t *testing.T) {
	sock, _ := startTestServer(t)

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Don't send hello; server should close after handshake timeout.
	_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout + 2*time.Second))
	_, err = readTestMsg(conn)
	// Should get an error (either a HANDSHAKE_TIMEOUT error msg or EOF).
	if err == nil {
		t.Error("expected error after handshake timeout, got nil")
	}
}

func TestPayloadTooLarge(t *testing.T) {
	sock, _ := startTestServer(t)

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	doHandshake(t, conn)

	// Write a length prefix claiming 11 MiB.
	var fakeLenBuf [4]byte
	binary.BigEndian.PutUint32(fakeLenBuf[:], 11<<20)
	_, _ = conn.Write(fakeLenBuf[:])
	// Write some data.
	_, _ = conn.Write(make([]byte, 1024))

	// Server should close the connection.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = readTestMsg(conn)
	if err == nil {
		t.Error("expected error for oversized message")
	}
}

func TestSocketPermissions(t *testing.T) {
	sock, _ := startTestServer(t)

	info, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	perm := info.Mode().Perm()
	// On macOS, socket files may not report exact permissions,
	// but we check the chmod was applied.
	if perm&0o077 != 0 {
		t.Errorf("socket permissions too open: %o", perm)
	}
}

func TestConnectionLimit(t *testing.T) {
	sock, _ := startTestServer(t)

	// Open maxConns connections.
	conns := make([]net.Conn, 0, maxConns)
	for i := 0; i < maxConns; i++ {
		c, err := net.Dial("unix", sock)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		doHandshake(t, c)
		conns = append(conns, c)
	}

	// The next connection should get OVERLOADED.
	extra, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial extra: %v", err)
	}
	defer func() { _ = extra.Close() }()

	_ = extra.SetReadDeadline(time.Now().Add(2 * time.Second))
	respBytes, err := readTestMsg(extra)
	if err != nil {
		// Connection may be immediately closed; that's acceptable.
		for _, c := range conns {
			_ = c.Close() // teardown
		}
		return
	}
	var resp msg
	_ = json.Unmarshal(respBytes, &resp)
	if resp.Code != "OVERLOADED" {
		t.Errorf("expected OVERLOADED, got %q", resp.Code)
	}

	for _, c := range conns {
		_ = c.Close()
	}
}

func TestReadWriteMsg(t *testing.T) {
	// Test the wire format: 4-byte big-endian length + payload.
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close() }()
	defer func() { _ = c2.Close() }()

	payload := []byte(`{"type":"hello"}`)
	go func() {
		if err := writeMsg(c1, payload); err != nil {
			t.Errorf("writeMsg: %v", err)
		}
	}()

	got, err := readMsg(c2)
	if err != nil {
		t.Fatalf("readMsg: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("payload mismatch: got %q, want %q", got, payload)
	}
}
