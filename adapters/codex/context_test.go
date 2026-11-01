package codex

import (
	"encoding/json"
	"testing"

	"hop.top/nerv/internal/merger"
)

func decodeOut(t *testing.T, out []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	return m
}

func TestFromEventDecision_Context_SupportedEvents(t *testing.T) {
	for _, ev := range []string{
		"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse",
	} {
		for _, a := range []merger.Action{merger.Allow, merger.Warn} {
			t.Run(ev+"/"+string(a), func(t *testing.T) {
				out, code, err := FromEventDecision(
					merger.Decision{Action: a, Message: "m", Context: "ctx"}, ev)
				if err != nil {
					t.Fatal(err)
				}
				if code != 0 {
					t.Fatalf("exit code = %d, want 0", code)
				}
				hso, ok := decodeOut(t, out)["hookSpecificOutput"].(map[string]any)
				if !ok {
					t.Fatalf("missing hookSpecificOutput in %q", out)
				}
				if hso["hookEventName"] != ev || hso["additionalContext"] != "ctx" {
					t.Fatalf("want hookEventName=%s additionalContext=ctx, got %v", ev, hso)
				}
				if _, has := hso["permissionDecision"]; has {
					t.Fatalf("allow/warn must not set permissionDecision: %q", out)
				}
			})
		}
	}
}

// Codex Stop/SubagentStop/compaction outputs reject unknown fields
// (hooks/src/schema.rs deny_unknown_fields), so context must not be
// emitted there even though Claude accepts it on Stop.
func TestFromEventDecision_Context_NoChannelEventsDropped(t *testing.T) {
	for _, ev := range []string{
		"Stop", "SubagentStop", "SessionEnd", "PermissionRequest",
		"PostToolUseFailure", "PreCompact", "PostCompact", "",
	} {
		t.Run(ev, func(t *testing.T) {
			out, _, err := FromEventDecision(
				merger.Decision{Action: merger.Allow, Context: "ctx"}, ev)
			if err != nil {
				t.Fatal(err)
			}
			if _, has := decodeOut(t, out)["hookSpecificOutput"]; has {
				t.Fatalf("%q has no codex context channel, got %q", ev, out)
			}
		})
	}
}

func TestFromEventDecision_Context_IgnoredOnBlock(t *testing.T) {
	out, code, err := FromEventDecision(
		merger.Decision{Action: merger.Block, Message: "no", Context: "ctx"}, "PreToolUse")
	if err != nil {
		t.Fatal(err)
	}
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	hso, _ := decodeOut(t, out)["hookSpecificOutput"].(map[string]any)
	if _, has := hso["additionalContext"]; has {
		t.Fatalf("block must not carry additionalContext: %q", out)
	}
}

func TestFromEventDecision_Context_RewriteKeepsUpdatedInput(t *testing.T) {
	out, _, err := FromEventDecision(merger.Decision{
		Action: merger.Rewrite, Rewrite: map[string]any{"command": "ls -la"}, Context: "ctx",
	}, "PreToolUse")
	if err != nil {
		t.Fatal(err)
	}
	hso, _ := decodeOut(t, out)["hookSpecificOutput"].(map[string]any)
	if hso["updatedInput"] == nil || hso["additionalContext"] != "ctx" {
		t.Fatalf("want updatedInput and additionalContext, got %q", out)
	}
}

func TestFromEventDecision_NoContext_Unchanged(t *testing.T) {
	out, code, err := FromEventDecision(merger.Decision{Action: merger.Allow}, "SessionStart")
	if err != nil || code != 0 || string(out) != "{}" {
		t.Fatalf("got %q, %d, %v; want {}, 0, nil", out, code, err)
	}
}
