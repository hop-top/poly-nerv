package eventlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/nerv/internal/envelope"
)

// TestJSONLLogger_DefaultDir pins the eventlog default to the
// internal events/ subtree of the state root — segregated from
// sessions/, which downstream crtx consumers scan.
func TestJSONLLogger_DefaultDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")

	l := &JSONLLogger{}
	want := filepath.Join("/custom/state", "nerv", "events")
	if got := l.dir(); got != want {
		t.Errorf("dir() = %q, want %q", got, want)
	}
}

// TestJSONLLogger_WriteLandsOutsideSessionScanRoot verifies a default
// logger writes its date-rotated Entry files under events/ and never
// under sessions/.
func TestJSONLLogger_WriteLandsOutsideSessionScanRoot(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	l := &JSONLLogger{}
	env := &envelope.Envelope{
		NervVersion: "1.0",
		Event:       "PreToolUse",
		EventID:     "ev-1",
		Timestamp:   time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
		CLI:         envelope.CLIInfo{Name: "claude", Version: "1.0.0"},
		Session:     envelope.SessionInfo{ID: "sess-1"},
		Payload:     json.RawMessage(`{}`),
	}
	if err := l.Write(env, map[string]any{"action": "allow"}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	path := filepath.Join(stateHome, "nerv", "events", "events-2026-08-07.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("entry file missing at %s: %v", path, err)
	}
	if _, err := os.Stat(filepath.Join(stateHome, "nerv", "sessions")); !os.IsNotExist(err) {
		t.Fatalf("eventlog write must not touch sessions/: %v", err)
	}
}
