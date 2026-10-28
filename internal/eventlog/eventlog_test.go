package eventlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/nerv/internal/envelope"
)

func makeEnv(event, cli, session string, ts time.Time) *envelope.Envelope {
	return &envelope.Envelope{
		NervVersion: "1.0.0-draft",
		Event:       event,
		EventID:     "test-id",
		Timestamp:   ts,
		CLI:         envelope.CLIInfo{Name: cli, Version: "1.0.0"},
		Session:     envelope.SessionInfo{ID: session, CWD: "/tmp"},
		Payload:     json.RawMessage(`{}`),
	}
}

func TestJSONLLogger_Write(t *testing.T) {
	dir := t.TempDir()
	logger := &JSONLLogger{Dir: dir}
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	env := makeEnv("SessionStart", "claude-code", "s1", ts)
	if err := logger.Write(env, map[string]string{"action": "allow"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Verify file exists with correct name.
	path := filepath.Join(dir, "events-2025-06-01.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("unmarshal entry: %v", err)
	}
	if entry.Envelope.Event != "SessionStart" {
		t.Errorf("event = %q, want SessionStart", entry.Envelope.Event)
	}
}

func TestJSONLLogger_Query_ByEvent(t *testing.T) {
	dir := t.TempDir()
	logger := &JSONLLogger{Dir: dir}
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	// Write two different events.
	_ = logger.Write(makeEnv("SessionStart", "claude-code", "s1", ts), nil)
	_ = logger.Write(makeEnv("PreToolUse", "claude-code", "s1", ts), nil)
	_ = logger.Write(makeEnv("SessionStart", "claude-code", "s2", ts), nil)

	entries, err := logger.Query(Filter{Event: "SessionStart"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	for _, e := range entries {
		if e.Envelope.Event != "SessionStart" {
			t.Errorf("unexpected event %q", e.Envelope.Event)
		}
	}
}

func TestJSONLLogger_Query_ByCLI(t *testing.T) {
	dir := t.TempDir()
	logger := &JSONLLogger{Dir: dir}
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	_ = logger.Write(makeEnv("SessionStart", "claude-code", "s1", ts), nil)
	_ = logger.Write(makeEnv("SessionStart", "codex-cli", "s2", ts), nil)
	_ = logger.Write(makeEnv("PreToolUse", "claude-code", "s3", ts), nil)

	entries, err := logger.Query(Filter{CLI: "codex-cli"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Envelope.CLI.Name != "codex-cli" {
		t.Errorf("cli = %q, want codex-cli", entries[0].Envelope.CLI.Name)
	}
}

func TestJSONLLogger_DateRotation(t *testing.T) {
	dir := t.TempDir()
	logger := &JSONLLogger{Dir: dir}

	day1 := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 6, 2, 10, 0, 0, 0, time.UTC)

	_ = logger.Write(makeEnv("SessionStart", "claude-code", "s1", day1), nil)
	_ = logger.Write(makeEnv("SessionStart", "claude-code", "s2", day2), nil)

	// Two separate files should exist.
	f1 := filepath.Join(dir, "events-2025-06-01.jsonl")
	f2 := filepath.Join(dir, "events-2025-06-02.jsonl")

	if _, err := os.Stat(f1); err != nil {
		t.Errorf("day1 file missing: %v", err)
	}
	if _, err := os.Stat(f2); err != nil {
		t.Errorf("day2 file missing: %v", err)
	}

	// Query should find both.
	entries, err := logger.Query(Filter{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
}
