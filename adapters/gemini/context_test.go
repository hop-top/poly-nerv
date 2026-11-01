package gemini

import (
	"testing"

	"hop.top/nerv/internal/merger"
)

func TestFromEventDecision_Context_SupportedEvents(t *testing.T) {
	for _, ev := range []string{"SessionStart", "BeforeAgent", "AfterTool"} {
		for _, a := range []merger.Action{merger.Allow, merger.Warn} {
			t.Run(ev+"/"+string(a), func(t *testing.T) {
				m, code, err := FromEventDecision(
					merger.Decision{Action: a, Message: "m", Context: "ctx"}, ev)
				if err != nil {
					t.Fatal(err)
				}
				if code != 0 {
					t.Fatalf("exit code = %d, want 0", code)
				}
				hso, ok := m["hookSpecificOutput"].(map[string]any)
				if !ok || hso["additionalContext"] != "ctx" {
					t.Fatalf("want hookSpecificOutput.additionalContext=ctx, got %v", m)
				}
			})
		}
	}
}

func TestFromEventDecision_Context_UnsupportedEventDropped(t *testing.T) {
	for _, ev := range []string{"BeforeTool", "AfterAgent", "SessionEnd", "BeforeModel", "PreCompress", ""} {
		t.Run(ev, func(t *testing.T) {
			m, _, err := FromEventDecision(
				merger.Decision{Action: merger.Allow, Context: "ctx"}, ev)
			if err != nil {
				t.Fatal(err)
			}
			if hso, ok := m["hookSpecificOutput"].(map[string]any); ok {
				if _, has := hso["additionalContext"]; has {
					t.Fatalf("%q has no context channel, got %v", ev, m)
				}
			}
		})
	}
}

func TestFromEventDecision_Context_RewriteKeepsToolInput(t *testing.T) {
	m, _, err := FromEventDecision(merger.Decision{
		Action: merger.Rewrite, Rewrite: map[string]any{"c": 1}, Context: "ctx",
	}, "AfterTool")
	if err != nil {
		t.Fatal(err)
	}
	hso := m["hookSpecificOutput"].(map[string]any)
	if hso["tool_input"] == nil || hso["additionalContext"] != "ctx" {
		t.Fatalf("got %v", hso)
	}
}

func TestFromEventDecision_Context_EmptyIsNoop(t *testing.T) {
	m, _, err := FromEventDecision(merger.Decision{Action: merger.Allow}, "SessionStart")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["hookSpecificOutput"]; ok {
		t.Fatalf("empty context must not emit hookSpecificOutput: %v", m)
	}
}

func TestFromDecision_ContextDroppedWithoutEvent(t *testing.T) {
	m, _, err := FromDecision(merger.Decision{Action: merger.Allow, Context: "ctx"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m["hookSpecificOutput"]; ok {
		t.Fatalf("event-less FromDecision has no context channel: %v", m)
	}
}

func TestFromEventDecision_Context_IgnoredOnBlock(t *testing.T) {
	m, code, err := FromEventDecision(
		merger.Decision{Action: merger.Block, Message: "no", Context: "ctx"}, "AfterTool")
	if err != nil {
		t.Fatal(err)
	}
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if _, ok := m["hookSpecificOutput"]; ok {
		t.Fatalf("block must not carry additionalContext: %v", m)
	}
}
