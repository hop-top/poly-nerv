//go:build !short

package vibe

import (
	"encoding/json"
	"strings"
	"testing"

	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// TestSmoke_EndToEnd exercises the full adapter pipeline:
// Vibe event -> ToEnvelope -> FromDecision.
func TestSmoke_EndToEnd(t *testing.T) {
	// 1. Simulate a Vibe UserMessageEvent.
	vibeEvent := map[string]any{
		"event":      "UserMessageEvent",
		"session_id": "smoke-sess",
		"cwd":        "/tmp/project",
		"timestamp":  "2026-04-11T12:00:00Z",
	}

	// 2. Translate to nerv envelope.
	env, err := ToEnvelope(vibeEvent)
	if err != nil {
		t.Fatalf("ToEnvelope: %v", err)
	}
	if env.Event != string(events.EventUserPromptSubmit) {
		t.Errorf("event = %q, want UserPromptSubmit", env.Event)
	}
	if env.Session.ID != "smoke-sess" {
		t.Errorf("session.id = %q", env.Session.ID)
	}

	// 3. Simulate a ToolCallEvent.
	toolEvent := map[string]any{
		"event":      "ToolCallEvent",
		"session_id": "smoke-sess",
		"tool_name":  "Write",
	}
	env2, err := ToEnvelope(toolEvent)
	if err != nil {
		t.Fatalf("ToEnvelope(ToolCallEvent): %v", err)
	}
	if env2.Event != string(events.EventPreToolUse) {
		t.Errorf("event = %q, want PreToolUse", env2.Event)
	}

	// 4. Convert a Decision back to Vibe format.
	decision := merger.Decision{
		Action:  merger.Block,
		Message: "blocked by policy",
	}
	m, exitCode, err := FromDecision(decision)
	if err != nil {
		t.Fatalf("FromDecision: %v", err)
	}
	if exitCode != 2 {
		t.Errorf("exit code = %d, want 2", exitCode)
	}
	if m["decision"] != "block" {
		t.Errorf("decision = %v, want block", m["decision"])
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
	if roundtrip["decision"] != "block" {
		t.Error("round-trip lost decision field")
	}
}

// TestSmoke_MergeAndDetect exercises adapter config and detection.
func TestSmoke_MergeAndDetect(t *testing.T) {
	a := &Adapter{}

	// Detect won't find vibe binary in test env; should not error.
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

	content := string(merged)
	if !strings.Contains(content, "[hooks]") {
		t.Error("missing [hooks] section")
	}
	if !strings.Contains(content, "nervd") {
		t.Error("missing nervd command in merged config")
	}
	if !strings.Contains(content, "ToolCallEvent") {
		t.Error("missing ToolCallEvent handler")
	}
	if !strings.Contains(content, "CompactStartEvent") {
		t.Error("missing CompactStartEvent handler")
	}
}

// TestSmoke_AllNativeEvents exercises translation of all native events.
func TestSmoke_AllNativeEvents(t *testing.T) {
	cases := []struct {
		name      string
		vibeEvent string
		nervEvent events.Event
	}{
		{"UserMessageEvent", "UserMessageEvent", events.EventUserPromptSubmit},
		{"AssistantEvent", "AssistantEvent", events.EventStop},
		{"ToolCallEvent", "ToolCallEvent", events.EventPreToolUse},
		{"ToolResultEvent", "ToolResultEvent", events.EventPostToolUse},
		{"WaitingForInputEvent", "WaitingForInputEvent", events.EventElicitation},
		{"CompactStartEvent", "CompactStartEvent", events.EventPreCompact},
		{"CompactEndEvent", "CompactEndEvent", events.EventPostCompact},
		{"AgentProfileChangedEvent", "AgentProfileChangedEvent", events.EventConfigChange},
		{"ApprovalCallback", "ApprovalCallback", events.EventPermissionRequest},
		{"SwitchAgentCallback", "SwitchAgentCallback", events.EventSubagentStart},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{
				"event":      tc.vibeEvent,
				"session_id": "test-sess",
			}
			env, err := ToEnvelope(input)
			if err != nil {
				t.Fatalf("ToEnvelope(%s): %v", tc.vibeEvent, err)
			}
			if env.Event != string(tc.nervEvent) {
				t.Errorf("event = %q, want %q", env.Event, tc.nervEvent)
			}
		})
	}
}
