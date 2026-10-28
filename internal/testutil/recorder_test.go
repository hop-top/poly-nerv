package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

func makeTestEnvelope(event, toolName string) *envelope.Envelope {
	payload, _ := json.Marshal(map[string]any{
		"tool_name":  toolName,
		"tool_input": map[string]any{"command": "echo test"},
	})
	return &envelope.Envelope{
		NervVersion: "1.0-draft",
		Event:       event,
		EventID:     "test-" + event + "-001",
		Timestamp:   time.Date(2026, 4, 11, 12, 0, 0, 0, time.UTC),
		CLI:         envelope.CLIInfo{Name: "claude", Version: "1.0.0"},
		Session:     envelope.SessionInfo{ID: "test-session"},
		Payload:     payload,
	}
}

func TestRecorder_Record_Flush(t *testing.T) {
	rec := NewRecorder()

	env := makeTestEnvelope("PreToolUse", "Bash")
	decision := merger.Decision{
		Action:  merger.Block,
		Message: "blocked for test",
	}

	rec.Record(env, decision)

	if len(rec.Entries()) != 1 {
		t.Fatalf("Entries() = %d; want 1", len(rec.Entries()))
	}

	// Flush to temp dir.
	tmp := t.TempDir()
	path := filepath.Join(tmp, "recorder_test.json")
	if err := rec.FlushTo(path); err != nil {
		t.Fatalf("FlushTo: %v", err)
	}

	// Read back and verify.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var entries []CassetteEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d; want 1", len(entries))
	}
	if entries[0].Output.Action != merger.Block {
		t.Errorf("Action = %q; want block", entries[0].Output.Action)
	}
	if entries[0].Output.Message != "blocked for test" {
		t.Errorf("Message = %q; want 'blocked for test'",
			entries[0].Output.Message)
	}
	if entries[0].Input.Event != "PreToolUse" {
		t.Errorf("Input.Event = %q; want PreToolUse",
			entries[0].Input.Event)
	}
}

func TestRecorder_Deterministic(t *testing.T) {
	// Same input recorded twice → same output file content
	// (except timestamps, which we normalize).
	env := makeTestEnvelope("SessionStart", "")
	decision := merger.Decision{Action: merger.Allow}

	rec1 := NewRecorder()
	rec1.Record(env, decision)

	rec2 := NewRecorder()
	rec2.Record(env, decision)

	tmp := t.TempDir()
	path1 := filepath.Join(tmp, "det1.json")
	path2 := filepath.Join(tmp, "det2.json")

	if err := rec1.FlushTo(path1); err != nil {
		t.Fatalf("FlushTo 1: %v", err)
	}
	if err := rec2.FlushTo(path2); err != nil {
		t.Fatalf("FlushTo 2: %v", err)
	}

	data1, err := os.ReadFile(path1)
	if err != nil {
		t.Fatalf("read cassette 1: %v", err)
	}
	data2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("read cassette 2: %v", err)
	}

	// Normalize: strip timestamps for comparison since they'll differ.
	var e1, e2 []CassetteEntry
	if err := json.Unmarshal(data1, &e1); err != nil {
		t.Fatalf("unmarshal cassette 1: %v", err)
	}
	if err := json.Unmarshal(data2, &e2); err != nil {
		t.Fatalf("unmarshal cassette 2: %v", err)
	}

	if len(e1) != len(e2) {
		t.Fatalf("entry count mismatch: %d vs %d", len(e1), len(e2))
	}

	for i := range e1 {
		if e1[i].Input.Event != e2[i].Input.Event {
			t.Errorf("[%d] input event mismatch: %q vs %q",
				i, e1[i].Input.Event, e2[i].Input.Event)
		}
		if e1[i].Output.Action != e2[i].Output.Action {
			t.Errorf("[%d] output action mismatch: %q vs %q",
				i, e1[i].Output.Action, e2[i].Output.Action)
		}
	}
}

func TestRecorder_Reset(t *testing.T) {
	rec := NewRecorder()
	rec.Record(
		makeTestEnvelope("PreToolUse", "Bash"),
		merger.Decision{Action: merger.Allow},
	)

	if len(rec.Entries()) != 1 {
		t.Fatal("expected 1 entry before reset")
	}

	rec.Reset()

	if len(rec.Entries()) != 0 {
		t.Error("expected 0 entries after reset")
	}
}

func TestRecorder_MultipleRecords(t *testing.T) {
	rec := NewRecorder()

	rec.Record(
		makeTestEnvelope("PreToolUse", "Bash"),
		merger.Decision{Action: merger.Block, Message: "blocked"},
	)
	rec.Record(
		makeTestEnvelope("SessionStart", ""),
		merger.Decision{Action: merger.Allow},
	)
	rec.Record(
		makeTestEnvelope("PostToolUse", "Read"),
		merger.Decision{Action: merger.Warn, Message: "caution"},
	)

	entries := rec.Entries()
	if len(entries) != 3 {
		t.Fatalf("Entries() = %d; want 3", len(entries))
	}

	// Verify order preserved.
	if entries[0].Output.Action != merger.Block {
		t.Error("entry[0] should be block")
	}
	if entries[1].Output.Action != merger.Allow {
		t.Error("entry[1] should be allow")
	}
	if entries[2].Output.Action != merger.Warn {
		t.Error("entry[2] should be warn")
	}
}
