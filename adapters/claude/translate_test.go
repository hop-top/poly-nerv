package claude

import (
	"encoding/json"
	"testing"
)

func TestToEnvelope_SessionStart(t *testing.T) {
	tr := &Translator{}

	event := map[string]any{
		"hook_event_name": "SessionStart",
		"session_id":      "sess-abc",
		"cwd":             "/home/user/project",
	}

	env, err := tr.ToEnvelope(event)
	if err != nil {
		t.Fatalf("ToEnvelope() error: %v", err)
	}
	if env.Event != "SessionStart" {
		t.Errorf("Event = %q, want %q", env.Event, "SessionStart")
	}
	if env.CLI.Name != "claude" {
		t.Errorf("CLI.Name = %q, want %q", env.CLI.Name, "claude")
	}
	if env.Synthesized {
		t.Error("Synthesized = true, want false")
	}
	if env.Session.ID != "sess-abc" {
		t.Errorf("Session.ID = %q, want %q", env.Session.ID, "sess-abc")
	}
	if env.Session.CWD != "/home/user/project" {
		t.Errorf(
			"Session.CWD = %q, want %q",
			env.Session.CWD, "/home/user/project",
		)
	}
}

func TestToEnvelope_PreToolUse(t *testing.T) {
	tr := &Translator{}

	event := map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "sess-xyz",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "ls -la"},
	}

	env, err := tr.ToEnvelope(event)
	if err != nil {
		t.Fatalf("ToEnvelope() error: %v", err)
	}
	if env.Event != "PreToolUse" {
		t.Errorf("Event = %q, want %q", env.Event, "PreToolUse")
	}

	// Verify payload contains tool_name and tool_input.
	var payload map[string]any
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["tool_name"] != "Bash" {
		t.Errorf(
			"payload.tool_name = %v, want %q",
			payload["tool_name"], "Bash",
		)
	}
	ti, ok := payload["tool_input"].(map[string]any)
	if !ok {
		t.Fatal("payload.tool_input not a map")
	}
	if ti["command"] != "ls -la" {
		t.Errorf(
			"payload.tool_input.command = %v, want %q",
			ti["command"], "ls -la",
		)
	}
}

func TestToEnvelope_MissingEventName(t *testing.T) {
	tr := &Translator{}

	_, err := tr.ToEnvelope(map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing hook_event_name")
	}
}

func TestToEnvelope_UnknownEvent(t *testing.T) {
	tr := &Translator{}

	_, err := tr.ToEnvelope(map[string]any{
		"hook_event_name": "BogusEvent",
	})
	if err == nil {
		t.Fatal("expected error for unknown event")
	}
}

func TestFromDecision_Allow(t *testing.T) {
	tr := &Translator{}

	out, code, err := tr.FromDecision(
		Decision{Action: "allow"}, "PreToolUse",
	)
	if err != nil {
		t.Fatalf("FromDecision() error: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if string(out) != "{}" {
		t.Errorf("output = %q, want %q", out, "{}")
	}
}

func TestFromDecision_Block(t *testing.T) {
	tr := &Translator{}

	out, code, err := tr.FromDecision(
		Decision{Action: "block", Reason: "dangerous command"},
		"PreToolUse",
	)
	if err != nil {
		t.Fatalf("FromDecision() error: %v", err)
	}
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	hso, ok := result["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatal("missing hookSpecificOutput")
	}
	if hso["permissionDecision"] != "deny" {
		t.Errorf(
			"permissionDecision = %v, want %q",
			hso["permissionDecision"], "deny",
		)
	}
	if hso["permissionDecisionReason"] != "dangerous command" {
		t.Errorf(
			"permissionDecisionReason = %v, want %q",
			hso["permissionDecisionReason"], "dangerous command",
		)
	}
}

func TestFromDecision_Block_PermissionRequest(t *testing.T) {
	tr := &Translator{}

	out, code, err := tr.FromDecision(
		Decision{Action: "block", Reason: "not allowed"},
		"PermissionRequest",
	)
	if err != nil {
		t.Fatalf("FromDecision() error: %v", err)
	}
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	hso, ok := result["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatal("missing hookSpecificOutput")
	}
	decision, ok := hso["decision"].(map[string]any)
	if !ok {
		t.Fatal("missing decision")
	}
	if decision["behavior"] != "deny" {
		t.Errorf("behavior = %v, want deny", decision["behavior"])
	}
}

func TestFromDecision_Rewrite(t *testing.T) {
	tr := &Translator{}

	modifiedInput := json.RawMessage(
		`{"command":"ls -la --no-color"}`,
	)
	out, code, err := tr.FromDecision(
		Decision{
			Action:  "rewrite",
			Payload: modifiedInput,
		},
		"PreToolUse",
	)
	if err != nil {
		t.Fatalf("FromDecision() error: %v", err)
	}
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	hso, ok := result["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatal("missing hookSpecificOutput")
	}
	if hso["permissionDecision"] != "allow" {
		t.Errorf(
			"permissionDecision = %v, want allow",
			hso["permissionDecision"],
		)
	}
	ui, ok := hso["updatedInput"].(map[string]any)
	if !ok {
		t.Fatal("missing updatedInput")
	}
	if ui["command"] != "ls -la --no-color" {
		t.Errorf(
			"updatedInput.command = %v, want %q",
			ui["command"], "ls -la --no-color",
		)
	}
}

func TestFromDecision_UnknownAction(t *testing.T) {
	tr := &Translator{}

	_, _, err := tr.FromDecision(
		Decision{Action: "noop"}, "PreToolUse",
	)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestFromDecision_Warn_DegradesToAllow(t *testing.T) {
	tr := &Translator{}

	out, code, err := tr.FromDecision(
		Decision{Action: "warn", Reason: "heads up"}, "PreToolUse",
	)
	if err != nil {
		t.Fatalf("FromDecision() error: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if string(out) != "{}" {
		t.Errorf("output = %q, want %q", out, "{}")
	}
}

// Worktree events carry a path contract on stdout (WorktreeCreate's
// stdout is read as a directory path), so the translator must stay
// silent for them no matter the decision.
func TestFromDecision_WorktreeEvents_Silent(t *testing.T) {
	tr := &Translator{}

	for _, event := range []string{"WorktreeCreate", "WorktreeRemove"} {
		for _, action := range []string{"allow", "warn", "block"} {
			out, code, err := tr.FromDecision(
				Decision{Action: action, Reason: "x"}, event,
			)
			if err != nil {
				t.Fatalf("%s/%s: error: %v", event, action, err)
			}
			if len(out) != 0 {
				t.Errorf(
					"%s/%s: output = %q, want empty",
					event, action, out,
				)
			}
			if code != 0 {
				t.Errorf(
					"%s/%s: exit code = %d, want 0",
					event, action, code,
				)
			}
		}
	}
}
