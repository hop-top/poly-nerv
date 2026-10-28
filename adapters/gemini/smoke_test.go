//go:build !short

package gemini

import (
	"encoding/json"
	"testing"

	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// TestSmoke_EndToEnd exercises the full adapter pipeline:
// Gemini event → ToEnvelope → synthesis → FromDecision.
func TestSmoke_EndToEnd(t *testing.T) {
	// 1. Simulate a Gemini BeforeTool event for a Shell command.
	geminiEvent := map[string]any{
		"hook_event_name": "BeforeTool",
		"session_id":      "smoke-sess",
		"cwd":             "/tmp/project",
		"timestamp":       "2026-04-11T12:00:00Z",
		"tool_name":       "Shell",
		"tool_input": map[string]any{
			"command": "cd /tmp/other && git worktree add feat-1",
		},
	}

	// 2. Translate to nerv envelope.
	env, err := ToEnvelope(geminiEvent)
	if err != nil {
		t.Fatalf("ToEnvelope: %v", err)
	}
	if env.Event != string(events.EventPreToolUse) {
		t.Errorf("event = %q, want PreToolUse", env.Event)
	}
	if env.Session.ID != "smoke-sess" {
		t.Errorf("session.id = %q", env.Session.ID)
	}

	// 3. Simulate AfterTool with synthesis.
	afterToolPayload := map[string]any{
		"tool_name": "Shell",
		"tool_input": map[string]any{
			"command": "cd /tmp/other && git worktree add feat-1",
		},
	}
	synth := SynthesizeFromAfterTool(afterToolPayload)

	// Expect CwdChanged + WorktreeCreate.
	if !hasSynthEvent(synth, events.EventCwdChanged) {
		t.Error("missing CwdChanged synthesis")
	}
	if !hasSynthEvent(synth, events.EventWorktreeCreate) {
		t.Error("missing WorktreeCreate synthesis")
	}

	// 4. Convert a Decision back to Gemini format.
	decision := merger.Decision{
		Action:  merger.Block,
		Message: "blocked by policy: unsafe worktree",
	}
	m, exitCode, err := FromDecision(decision)
	if err != nil {
		t.Fatalf("FromDecision: %v", err)
	}
	if exitCode != 2 {
		t.Errorf("exit code = %d, want 2", exitCode)
	}
	if m["decision"] != "deny" {
		t.Errorf("decision = %v, want deny", m["decision"])
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
	if roundtrip["decision"] != "deny" {
		t.Error("round-trip lost decision field")
	}
}

// TestSmoke_MergeAndDetect exercises adapter config and detection.
func TestSmoke_MergeAndDetect(t *testing.T) {
	a := &Adapter{}

	// Detect won't find gemini binary in test env; should not error.
	res, err := a.Detect()
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	// We don't assert Found since gemini may or may not be installed.
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

	hooks := cfg["hooks"].(map[string]any)

	// Verify BeforeTool entry structure.
	bt, ok := hooks["BeforeTool"]
	if !ok {
		t.Fatal("missing BeforeTool in merged config")
	}

	entries, ok := bt.([]any)
	if !ok || len(entries) == 0 {
		t.Fatal("BeforeTool should be non-empty array")
	}

	entry := entries[0].(map[string]any)
	hooksList := entry["hooks"].([]any)
	hook := hooksList[0].(map[string]any)

	if hook["type"] != "command" {
		t.Errorf("hook type = %v, want command", hook["type"])
	}
	if hook["command"] != "nerv-adapter gemini BeforeTool" {
		t.Errorf("hook command = %v", hook["command"])
	}
}

// TestSmoke_SynthesisPipeline exercises synthesis for all patterns.
func TestSmoke_SynthesisPipeline(t *testing.T) {
	cases := []struct {
		name   string
		event  string // Gemini event name
		fn     func(map[string]any) []SyntheticEvent
		input  map[string]any
		expect events.Event
	}{
		{
			name:  "BeforeTool TaskCreate → TaskCreated",
			event: "BeforeTool",
			fn:    SynthesizeFromBeforeTool,
			input: map[string]any{
				"tool_name":  "TaskCreate",
				"tool_input": map[string]any{"title": "test"},
			},
			expect: events.EventTaskCreated,
		},
		{
			name:  "AfterTool Read AGENTS.md → InstructionsLoaded",
			event: "AfterTool",
			fn:    SynthesizeFromAfterTool,
			input: map[string]any{
				"tool_name":  "Read",
				"tool_input": map[string]any{"path": "AGENTS.md"},
			},
			expect: events.EventInstructionsLoaded,
		},
		{
			name:  "AfterTool Edit → FileChanged",
			event: "AfterTool",
			fn:    SynthesizeFromAfterTool,
			input: map[string]any{
				"tool_name":  "Edit",
				"tool_input": map[string]any{"path": "main.go"},
			},
			expect: events.EventFileChanged,
		},
		{
			name:  "AfterTool error → PostToolUseFailure",
			event: "AfterTool",
			fn:    SynthesizeFromAfterTool,
			input: map[string]any{
				"tool_name": "Shell",
				"tool_input": map[string]any{
					"command": "exit 1",
				},
				"tool_response": map[string]any{
					"error": "non-zero exit",
				},
			},
			expect: events.EventPostToolUseFailure,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synth := tc.fn(tc.input)
			if !hasSynthEvent(synth, tc.expect) {
				t.Errorf("expected %s synthesis", tc.expect)
			}
		})
	}
}
