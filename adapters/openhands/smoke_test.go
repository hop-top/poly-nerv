//go:build !short

package openhands

import (
	"encoding/json"
	"testing"

	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// TestSmoke_EndToEnd exercises the full adapter pipeline:
// OpenHands event -> ToEnvelope -> synthesis -> FromDecision.
func TestSmoke_EndToEnd(t *testing.T) {
	// 1. Simulate an OpenHands PreToolUse event.
	ohEvent := map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "smoke-sess",
		"cwd":             "/tmp/project",
		"timestamp":       "2026-04-11T12:00:00Z",
		"tool_name":       "Bash",
	}

	// 2. Translate to nerv envelope.
	env, err := ToEnvelope(ohEvent)
	if err != nil {
		t.Fatalf("ToEnvelope: %v", err)
	}
	if env.Event != string(events.EventPreToolUse) {
		t.Errorf("event = %q, want PreToolUse", env.Event)
	}
	if env.Session.ID != "smoke-sess" {
		t.Errorf("session.id = %q", env.Session.ID)
	}

	// 3. Simulate event stream synthesis.
	streamPayload := map[string]any{
		"action":  "CmdRunAction",
		"command": "cd /tmp/other",
	}
	synth := SynthesizeFromEventStream(streamPayload)
	if !hasSynthEvent(synth, events.EventCwdChanged) {
		t.Error("missing CwdChanged synthesis")
	}

	// 4. Convert a Decision back to OpenHands format.
	decision := merger.Decision{
		Action:  merger.Block,
		Message: "blocked by policy: unsafe command",
	}
	m, exitCode, err := FromDecision(decision)
	if err != nil {
		t.Fatalf("FromDecision: %v", err)
	}
	if exitCode != 2 {
		t.Errorf("exit code = %d, want 2", exitCode)
	}
	if m["action"] != "block" {
		t.Errorf("action = %v, want block", m["action"])
	}

	// 5. Verify JSON round-trip.
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip map[string]any
	if err := json.Unmarshal(raw, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if roundtrip["action"] != "block" {
		t.Error("round-trip lost action field")
	}
}

// TestSmoke_MergeAndDetect exercises adapter config and detection.
func TestSmoke_MergeAndDetect(t *testing.T) {
	a := &Adapter{}

	// Detect won't find openhands binary in test env; should not error.
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

	var cfg map[string]any
	if err := json.Unmarshal(merged, &cfg); err != nil {
		t.Fatalf("unmarshal merged config: %v", err)
	}

	hooks, ok := cfg["hooks"].(map[string]any)
	if !ok {
		t.Fatal("missing hooks in merged config")
	}

	// Verify native events are registered.
	for _, event := range []string{
		"PreToolUse", "PostToolUse", "UserPromptSubmit",
		"SessionStart", "SessionEnd", "Stop",
	} {
		if _, ok := hooks[event]; !ok {
			t.Errorf("missing %s in merged config", event)
		}
	}
}

// TestSmoke_SynthesisPipeline exercises synthesis for all patterns.
func TestSmoke_SynthesisPipeline(t *testing.T) {
	cases := []struct {
		name   string
		input  map[string]any
		expect events.Event
	}{
		{
			name: "AgentDelegateAction -> SubagentStart",
			input: map[string]any{
				"action": "AgentDelegateAction",
				"agent":  "sub-1",
			},
			expect: events.EventSubagentStart,
		},
		{
			name: "CmdRunAction cd -> CwdChanged",
			input: map[string]any{
				"action":  "CmdRunAction",
				"command": "cd /tmp",
			},
			expect: events.EventCwdChanged,
		},
		{
			name: "ErrorObservation -> PostToolUseFailure",
			input: map[string]any{
				"observation": "ErrorObservation",
				"error":       "non-zero exit",
			},
			expect: events.EventPostToolUseFailure,
		},
		{
			name: "CondensationAction -> PreCompact",
			input: map[string]any{
				"action": "CondensationAction",
			},
			expect: events.EventPreCompact,
		},
		{
			name: "UserRejectObservation -> PermissionDenied",
			input: map[string]any{
				"observation": "UserRejectObservation",
			},
			expect: events.EventPermissionDenied,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synth := SynthesizeFromEventStream(tc.input)
			if !hasSynthEvent(synth, tc.expect) {
				t.Errorf("expected %s synthesis", tc.expect)
			}
		})
	}
}
