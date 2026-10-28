//go:build !short

package goose

import (
	"encoding/json"
	"testing"

	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// TestSmoke_EndToEnd exercises the full adapter pipeline:
// Goose event -> ToEnvelope -> FromDecision across native and
// synthesized events.
func TestSmoke_EndToEnd(t *testing.T) {
	// 1. Native SSE event: finish -> Stop.
	finishEvent := map[string]any{
		"type":       "finish",
		"session_id": "smoke-sess",
		"reason":     "complete",
	}
	env, err := ToEnvelope(finishEvent)
	if err != nil {
		t.Fatalf("ToEnvelope(finish): %v", err)
	}
	if env.Event != string(events.EventStop) {
		t.Errorf("event = %q, want Stop", env.Event)
	}
	if env.Synthesized {
		t.Error("finish should be native (not synthesized)")
	}

	// 2. Native permission event: action_required -> PermissionRequest.
	permEvent := map[string]any{
		"type":      "action_required",
		"tool_name": "shell",
	}
	permEnv, err := ToEnvelope(permEvent)
	if err != nil {
		t.Fatalf("ToEnvelope(action_required): %v", err)
	}
	if permEnv.Event != string(events.EventPermissionRequest) {
		t.Errorf("event = %q, want PermissionRequest", permEnv.Event)
	}
	if permEnv.Synthesized {
		t.Error("action_required should be native")
	}

	// 3. Native subagent event.
	subEvent := map[string]any{
		"type":       "subagent_created",
		"session_id": "smoke-sess",
		"task_id":    "task-42",
	}
	subEnv, err := ToEnvelope(subEvent)
	if err != nil {
		t.Fatalf("ToEnvelope(subagent_created): %v", err)
	}
	if subEnv.Event != string(events.EventSubagentStart) {
		t.Errorf("event = %q, want SubagentStart", subEnv.Event)
	}

	// 4. Synthesized extension event: tool.before -> PreToolUse.
	toolEvent := map[string]any{
		"type":       "tool.before",
		"session_id": "smoke-sess",
		"cwd":        "/tmp/project",
		"timestamp":  "2026-04-11T12:00:00Z",
		"tool_name":  "shell",
		"tool_input": map[string]any{
			"command": "ls -la",
		},
	}
	toolEnv, err := ToEnvelope(toolEvent)
	if err != nil {
		t.Fatalf("ToEnvelope(tool.before): %v", err)
	}
	if toolEnv.Event != string(events.EventPreToolUse) {
		t.Errorf("event = %q, want PreToolUse", toolEnv.Event)
	}
	if !toolEnv.Synthesized {
		t.Error("tool.before should be synthesized")
	}
	if toolEnv.Session.ID != "smoke-sess" {
		t.Errorf("session.id = %q", toolEnv.Session.ID)
	}

	// 5. Convert Decision with block -> exit code 2.
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
	if m["action"] != "cancel" {
		t.Errorf("action = %v, want cancel", m["action"])
	}

	// 6. Verify JSON round-trip.
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip map[string]any
	if err := json.Unmarshal(raw, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if roundtrip["action"] != "cancel" {
		t.Error("round-trip lost action field")
	}
}

// TestSmoke_NativeVsSynthesized verifies classification of all event
// types across the native/synthesized boundary.
func TestSmoke_NativeVsSynthesized(t *testing.T) {
	native := []string{
		"session.start", "session.end", "finish",
		"notification", "update_conversation",
		"action_required", "permission_denied",
		"subagent_created", "subagent_completed",
		"subagent_terminated", "tasks_complete", "error",
	}
	synthesized := []string{
		"tool.before", "tool.after", "tool.error",
		"file.changed", "cwd.changed",
		"worktree.created", "worktree.removed",
		"compaction.request", "instructions.loaded",
		"request.start", "tasks_update",
	}

	for _, name := range native {
		ev := map[string]any{"type": name}
		env, err := ToEnvelope(ev)
		if err != nil {
			t.Errorf("native %q: %v", name, err)
			continue
		}
		if env.Synthesized {
			t.Errorf("%q should be native, got synthesized", name)
		}
	}

	for _, name := range synthesized {
		ev := map[string]any{"type": name}
		env, err := ToEnvelope(ev)
		if err != nil {
			t.Errorf("synthesized %q: %v", name, err)
			continue
		}
		if !env.Synthesized {
			t.Errorf("%q should be synthesized, got native", name)
		}
	}
}

// TestSmoke_MergeAndDetect exercises adapter config and detection.
func TestSmoke_MergeAndDetect(t *testing.T) {
	a := &Adapter{}

	// Detect won't find goose binary in test env; should not error.
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
	if !containsStr(content, "nerv-bridge") {
		t.Error("merged config missing nerv-bridge extension")
	}
	if !containsStr(content, "nerv") {
		t.Error("merged config missing nerv command")
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
		t.Errorf("merge not idempotent:\nfirst:  %s\nsecond: %s",
			first, second)
	}
}

// TestSmoke_Capabilities verifies the capability map.
func TestSmoke_Capabilities(t *testing.T) {
	a := &Adapter{}
	caps := a.Capabilities()

	if !caps.PreToolUse {
		t.Error("PreToolUse should be true")
	}
	if !caps.PostToolUse {
		t.Error("PostToolUse should be true")
	}
	if !caps.PreSession {
		t.Error("PreSession should be true")
	}
	if !caps.PostSession {
		t.Error("PostSession should be true")
	}
	if !caps.Streaming {
		t.Error("Streaming should be true (SSE support)")
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
