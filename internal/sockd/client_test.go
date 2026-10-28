//go:build !windows

package sockd

import (
	"encoding/json"
	"testing"
)

func TestClientDialAndSend(t *testing.T) {
	sock, _ := startTestServer(t)

	c, err := Dial(sock, "claude", "1.0", "sess_test")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	env := json.RawMessage(`{
		"event": "PreToolUse",
		"cli": {"name": "claude", "version": "1.0"},
		"session": {"id": "sess_test"},
		"timestamp": "2026-04-11T12:00:00Z"
	}`)

	dec, err := c.Send("evt_001", env)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var d map[string]any
	if err := json.Unmarshal(dec, &d); err != nil {
		t.Fatalf("unmarshal decision: %v", err)
	}
	if d["action"] != "allow" {
		t.Errorf("expected allow, got %v", d["action"])
	}
}

func TestClientMultipleEnvelopes(t *testing.T) {
	sock, _ := startTestServer(t)

	c, err := Dial(sock, "claude", "1.0", "sess_multi")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	for i := range 5 {
		env := json.RawMessage(`{"event": "PreToolUse"}`)
		_, err := c.Send("evt_"+string(rune('0'+i)), env)
		if err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}
}

func TestSocketAvailableNoSocket(t *testing.T) {
	t.Setenv("NERV_SOCKET", "/tmp/nonexistent-nerv-test.sock")
	_, ok := SocketAvailable()
	if ok {
		t.Error("expected unavailable for nonexistent socket")
	}
}

func TestSocketAvailableWithServer(t *testing.T) {
	sock, _ := startTestServer(t)
	t.Setenv("NERV_SOCKET", sock)
	path, ok := SocketAvailable()
	if !ok {
		t.Error("expected available with running server")
	}
	if path != sock {
		t.Errorf("path mismatch: got %q, want %q", path, sock)
	}
}
