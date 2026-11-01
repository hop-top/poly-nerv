package openhands

import (
	"testing"

	"hop.top/nerv/internal/merger"
)

func TestFromEventDecision_Context_SupportedEvents(t *testing.T) {
	for _, ev := range []string{"SessionStart", "UserPromptSubmit"} {
		for _, a := range []merger.Action{merger.Allow, merger.Warn, merger.Rewrite} {
			t.Run(ev+"/"+string(a), func(t *testing.T) {
				m, code, err := FromEventDecision(
					merger.Decision{Action: a, Context: "ctx"}, ev)
				if err != nil {
					t.Fatal(err)
				}
				if code != 0 {
					t.Fatalf("exit code = %d, want 0", code)
				}
				// OpenHands reads additionalContext at the top level of
				// the hook's stdout JSON (hooks/executor.py).
				if m["additionalContext"] != "ctx" {
					t.Fatalf("want top-level additionalContext=ctx, got %v", m)
				}
			})
		}
	}
}

func TestFromEventDecision_Context_NoChannelEventsDropped(t *testing.T) {
	for _, ev := range []string{
		"PreToolUse", "PostToolUse", "Stop", "SessionEnd", "",
	} {
		t.Run(ev, func(t *testing.T) {
			m, _, err := FromEventDecision(
				merger.Decision{Action: merger.Allow, Context: "ctx"}, ev)
			if err != nil {
				t.Fatal(err)
			}
			if _, has := m["additionalContext"]; has {
				t.Fatalf("%q has no context channel, got %v", ev, m)
			}
		})
	}
}

func TestFromEventDecision_Context_DroppedOnBlock(t *testing.T) {
	m, code, err := FromEventDecision(
		merger.Decision{Action: merger.Block, Message: "no", Context: "ctx"},
		"UserPromptSubmit")
	if err != nil {
		t.Fatal(err)
	}
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if _, has := m["additionalContext"]; has {
		t.Fatalf("block must not carry additionalContext: %v", m)
	}
}

func TestFromDecision_DropsContext(t *testing.T) {
	m, _, err := FromDecision(merger.Decision{Action: merger.Allow, Context: "ctx"})
	if err != nil {
		t.Fatal(err)
	}
	if _, has := m["additionalContext"]; has {
		t.Fatalf("FromDecision has no event, must drop context: %v", m)
	}
}
