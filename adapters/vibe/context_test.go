package vibe

import (
	"testing"

	"hop.top/nerv/internal/merger"
)

func TestFromEventDecision_Context_PostToolOnly(t *testing.T) {
	// Vibe reads hook_specific_output.additional_context on post_tool
	// hooks only (native ToolResultEvent / PostToolUse).
	for _, ev := range []string{"ToolResultEvent", "post_tool"} {
		for _, a := range []merger.Action{merger.Allow, merger.Warn} {
			t.Run(ev+"/"+string(a), func(t *testing.T) {
				m, code, err := FromEventDecision(
					merger.Decision{Action: a, Context: "ctx"}, ev)
				if err != nil {
					t.Fatal(err)
				}
				if code != 0 {
					t.Fatalf("exit code = %d, want 0", code)
				}
				hso, ok := m["hook_specific_output"].(map[string]any)
				if !ok || hso["additional_context"] != "ctx" {
					t.Fatalf("want hook_specific_output.additional_context=ctx, got %v", m)
				}
			})
		}
	}
}

func TestFromEventDecision_Context_NoChannelEventsDropped(t *testing.T) {
	for _, ev := range []string{
		"ToolCallEvent", "pre_tool", "post_agent", "UserMessageEvent",
		"AssistantEvent", "CompactStartEvent", "",
	} {
		t.Run(ev, func(t *testing.T) {
			m, _, err := FromEventDecision(
				merger.Decision{Action: merger.Allow, Context: "ctx"}, ev)
			if err != nil {
				t.Fatal(err)
			}
			if _, has := m["hook_specific_output"]; has {
				t.Fatalf("%q has no context channel, got %v", ev, m)
			}
		})
	}
}

func TestFromEventDecision_Context_DroppedOnBlock(t *testing.T) {
	m, code, err := FromEventDecision(
		merger.Decision{Action: merger.Block, Message: "no", Context: "ctx"},
		"ToolResultEvent")
	if err != nil {
		t.Fatal(err)
	}
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if _, has := m["hook_specific_output"]; has {
		t.Fatalf("block must not carry context: %v", m)
	}
}

func TestFromDecision_DropsContext(t *testing.T) {
	m, _, err := FromDecision(merger.Decision{Action: merger.Allow, Context: "ctx"})
	if err != nil {
		t.Fatal(err)
	}
	if _, has := m["hook_specific_output"]; has {
		t.Fatalf("FromDecision has no event, must drop context: %v", m)
	}
}
