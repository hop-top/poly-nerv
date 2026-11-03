package claude

import (
	"encoding/json"
	"testing"
)

// contextEventsUnderTest mirrors the Claude Code events documented to
// accept hookSpecificOutput.additionalContext.
var contextEventsUnderTest = []string{
	"SessionStart", "SubagentStart", "UserPromptSubmit", "PreToolUse",
	"PostToolUse", "PostToolUseFailure", "Stop", "SubagentStop",
}

func decodeHSO(t *testing.T, out []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	hso, ok := m["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("missing hookSpecificOutput in %q", out)
	}
	return hso
}

func TestFromDecision_Context_SupportedEvents(t *testing.T) {
	tr := &Translator{}
	for _, ev := range contextEventsUnderTest {
		for _, action := range []string{"allow", "warn"} {
			t.Run(ev+"/"+action, func(t *testing.T) {
				out, code, err := tr.FromDecision(
					Decision{Action: action, Reason: "r", Context: "ctx text"}, ev)
				if err != nil {
					t.Fatal(err)
				}
				if code != 0 {
					t.Fatalf("exit code = %d, want 0", code)
				}
				hso := decodeHSO(t, out)
				if hso["hookEventName"] != ev {
					t.Errorf("hookEventName = %v, want %s", hso["hookEventName"], ev)
				}
				if hso["additionalContext"] != "ctx text" {
					t.Errorf("additionalContext = %v", hso["additionalContext"])
				}
				if _, ok := hso["permissionDecision"]; ok {
					t.Errorf("allow/warn must not set permissionDecision: %q", out)
				}
			})
		}
	}
}

func TestFromDecision_Context_UnsupportedEventDropped(t *testing.T) {
	tr := &Translator{}
	for _, ev := range []string{"SessionEnd", "Notification", "PreCompact", "PermissionRequest"} {
		t.Run(ev, func(t *testing.T) {
			out, code, err := tr.FromDecision(
				Decision{Action: "allow", Context: "ctx"}, ev)
			if err != nil {
				t.Fatal(err)
			}
			if code != 0 || string(out) != "{}" {
				t.Fatalf("got %q exit %d, want {} exit 0", out, code)
			}
		})
	}
}

func TestFromDecision_Context_EmptyIsNoop(t *testing.T) {
	tr := &Translator{}
	out, code, err := tr.FromDecision(Decision{Action: "allow"}, "SessionStart")
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || string(out) != "{}" {
		t.Fatalf("got %q exit %d, want {} exit 0", out, code)
	}
}

func TestFromDecision_Context_SilentEventsStaySilent(t *testing.T) {
	tr := &Translator{}
	out, code, err := tr.FromDecision(
		Decision{Action: "allow", Context: "ctx"}, "WorktreeCreate")
	if err != nil {
		t.Fatal(err)
	}
	if out != nil || code != 0 {
		t.Fatalf("got %q exit %d, want nil exit 0", out, code)
	}
}

func TestFromDecision_Context_IgnoredOnBlock(t *testing.T) {
	tr := &Translator{}
	out, code, err := tr.FromDecision(
		Decision{Action: "block", Reason: "no", Context: "ctx"}, "PreToolUse")
	if err != nil {
		t.Fatal(err)
	}
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if _, ok := decodeHSO(t, out)["additionalContext"]; ok {
		t.Fatalf("block must not carry additionalContext: %q", out)
	}
}

func TestFromDecision_Context_WithRewrite_PreToolUse(t *testing.T) {
	tr := &Translator{}
	out, code, err := tr.FromDecision(Decision{
		Action:  "rewrite",
		Payload: json.RawMessage(`{"tool_input":{"command":"ls -la"}}`),
		Context: "ctx",
	}, "PreToolUse")
	if err != nil {
		t.Fatal(err)
	}
	if code != 3 {
		t.Fatalf("exit code = %d, want 3 (unchanged)", code)
	}
	hso := decodeHSO(t, out)
	if hso["permissionDecision"] != "allow" || hso["updatedInput"] == nil {
		t.Fatalf("rewrite output lost: %q", out)
	}
	if hso["additionalContext"] != "ctx" {
		t.Fatalf("additionalContext = %v, want ctx", hso["additionalContext"])
	}
}

func TestFromDecision_Context_WithRewrite_PermissionRequestDropped(t *testing.T) {
	tr := &Translator{}
	out, _, err := tr.FromDecision(Decision{
		Action:  "rewrite",
		Payload: json.RawMessage(`{"tool_input":{"command":"ls"}}`),
		Context: "ctx",
	}, "PermissionRequest")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decodeHSO(t, out)["additionalContext"]; ok {
		t.Fatalf("PermissionRequest has no context channel: %q", out)
	}
}
