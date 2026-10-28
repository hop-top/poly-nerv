//go:build !short

package aider

import (
	"encoding/json"
	"testing"

	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// TestSmoke_EndToEnd exercises the full adapter pipeline:
// Aider event -> ToEnvelope -> FromDecision.
func TestSmoke_EndToEnd(t *testing.T) {
	// 1. Simulate an Aider bridge PreToolUse event.
	aiderEvent := map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "smoke-sess",
		"cwd":             "/tmp/project",
		"timestamp":       "2026-04-11T12:00:00Z",
		"tool_name":       "edit",
		"tool_input": map[string]any{
			"file": "main.py",
		},
	}

	// 2. Translate to nerv envelope.
	env, err := ToEnvelope(aiderEvent)
	if err != nil {
		t.Fatalf("ToEnvelope: %v", err)
	}
	if env.Event != string(events.EventPreToolUse) {
		t.Errorf("event = %q, want PreToolUse", env.Event)
	}
	if env.Session.ID != "smoke-sess" {
		t.Errorf("session.id = %q", env.Session.ID)
	}
	if !env.Synthesized {
		t.Error("expected Synthesized=true for aider events")
	}

	// 3. Convert a Decision back to Aider format.
	decision := merger.Decision{
		Action:  merger.Block,
		Message: "blocked by policy",
	}
	m, exitCode, err := FromDecision(decision)
	if err != nil {
		t.Fatalf("FromDecision: %v", err)
	}
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if m["decision"] != "block" {
		t.Errorf("decision = %v, want block", m["decision"])
	}

	// 4. Verify JSON round-trip.
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip map[string]any
	if err := json.Unmarshal(raw, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if roundtrip["decision"] != "block" {
		t.Error("round-trip lost decision field")
	}
}

// TestSmoke_MergeAndDetect exercises adapter config and detection.
func TestSmoke_MergeAndDetect(t *testing.T) {
	a := &Adapter{}

	// Detect won't find aider binary in test env; should not error.
	res, err := a.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	_ = res

	// Merge into empty config.
	merged, err := a.MergeConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) == 0 {
		t.Fatal("MergeConfig(nil) returned empty")
	}

	content := string(merged)
	if !containsStr(content, "nerv dispatch aider") {
		t.Error("merged config missing nerv dispatch command")
	}
	if !containsStr(content, "lint-cmd") {
		t.Error("merged config missing lint-cmd")
	}
}

// TestSmoke_MergeIdempotent verifies merge is idempotent.
func TestSmoke_MergeIdempotent(t *testing.T) {
	a := &Adapter{}

	first, err := a.MergeConfig(nil)
	if err != nil {
		t.Fatal(err)
	}

	second, err := a.MergeConfig(first)
	if err != nil {
		t.Fatal(err)
	}

	if string(first) != string(second) {
		t.Errorf("merge is not idempotent:\nfirst:  %s\nsecond: %s",
			first, second)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
