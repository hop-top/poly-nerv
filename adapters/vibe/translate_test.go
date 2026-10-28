package vibe

import (
	"testing"

	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

func TestToEnvelope_NativeEvents(t *testing.T) {
	tests := []struct {
		name      string
		input     map[string]any
		wantEvent string
		wantErr   bool
	}{
		{
			name: "UserMessageEvent maps to UserPromptSubmit",
			input: map[string]any{
				"event":      "UserMessageEvent",
				"session_id": "sess-1",
				"cwd":        "/tmp",
			},
			wantEvent: "UserPromptSubmit",
		},
		{
			name: "AssistantEvent maps to Stop",
			input: map[string]any{
				"event":      "AssistantEvent",
				"session_id": "sess-2",
			},
			wantEvent: "Stop",
		},
		{
			name: "ToolCallEvent maps to PreToolUse",
			input: map[string]any{
				"event":      "ToolCallEvent",
				"session_id": "sess-3",
				"tool_name":  "Write",
			},
			wantEvent: "PreToolUse",
		},
		{
			name: "ToolResultEvent maps to PostToolUse",
			input: map[string]any{
				"event":      "ToolResultEvent",
				"session_id": "sess-4",
			},
			wantEvent: "PostToolUse",
		},
		{
			name: "WaitingForInputEvent maps to Elicitation",
			input: map[string]any{
				"event":      "WaitingForInputEvent",
				"session_id": "sess-5",
			},
			wantEvent: "Elicitation",
		},
		{
			name: "CompactStartEvent maps to PreCompact",
			input: map[string]any{
				"event": "CompactStartEvent",
			},
			wantEvent: "PreCompact",
		},
		{
			name: "CompactEndEvent maps to PostCompact",
			input: map[string]any{
				"event": "CompactEndEvent",
			},
			wantEvent: "PostCompact",
		},
		{
			name: "AgentProfileChangedEvent maps to ConfigChange",
			input: map[string]any{
				"event": "AgentProfileChangedEvent",
			},
			wantEvent: "ConfigChange",
		},
		{
			name: "ApprovalCallback maps to PermissionRequest",
			input: map[string]any{
				"event": "ApprovalCallback",
			},
			wantEvent: "PermissionRequest",
		},
		{
			name: "SwitchAgentCallback maps to SubagentStart",
			input: map[string]any{
				"event": "SwitchAgentCallback",
			},
			wantEvent: "SubagentStart",
		},
		{
			name:    "missing event field",
			input:   map[string]any{},
			wantErr: true,
		},
		{
			name: "unknown event",
			input: map[string]any{
				"event": "NoSuchEvent",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := ToEnvelope(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if env.Event != tt.wantEvent {
				t.Errorf("event = %q, want %q", env.Event, tt.wantEvent)
			}
			if env.CLI.Name != "vibe" {
				t.Errorf("cli.name = %q, want %q", env.CLI.Name, "vibe")
			}
		})
	}
}

func TestToEnvelope_SessionInfo(t *testing.T) {
	env, err := ToEnvelope(map[string]any{
		"event":      "UserMessageEvent",
		"session_id": "abc-123",
		"cwd":        "/home/user/project",
		"timestamp":  "2026-04-11T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if env.Session.ID != "abc-123" {
		t.Errorf("session.id = %q, want %q", env.Session.ID, "abc-123")
	}
	if env.Session.CWD != "/home/user/project" {
		t.Errorf("session.cwd = %q", env.Session.CWD)
	}
}

func TestFromDecision_Allow(t *testing.T) {
	m, code, err := FromDecision(merger.Decision{Action: merger.Allow})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if m["decision"] != "allow" {
		t.Errorf("decision = %v, want allow", m["decision"])
	}
}

func TestFromDecision_Block(t *testing.T) {
	m, code, err := FromDecision(merger.Decision{
		Action:  merger.Block,
		Message: "blocked by policy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if m["decision"] != "block" {
		t.Errorf("decision = %v, want block", m["decision"])
	}
	if m["reason"] != "blocked by policy" {
		t.Errorf("reason = %v", m["reason"])
	}
}

func TestFromDecision_Warn(t *testing.T) {
	m, code, err := FromDecision(merger.Decision{
		Action:  merger.Warn,
		Message: "caution",
	})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if m["decision"] != "allow" {
		t.Errorf("decision = %v, want allow (warn->allow)", m["decision"])
	}
}

func TestTranslateEventName(t *testing.T) {
	if got := TranslateEventName("ToolCallEvent"); got != "PreToolUse" {
		t.Errorf("got %q, want PreToolUse", got)
	}
	if got := TranslateEventName("Unknown"); got != "Unknown" {
		t.Errorf("got %q, want Unknown passthrough", got)
	}
}

func TestReverseEventName(t *testing.T) {
	if got := ReverseEventName(events.EventPreToolUse); got != "ToolCallEvent" {
		t.Errorf("got %q, want ToolCallEvent", got)
	}
	if got := ReverseEventName(events.EventTeammateIdle); got != "" {
		t.Errorf("got %q, want empty (unsupported)", got)
	}
}
