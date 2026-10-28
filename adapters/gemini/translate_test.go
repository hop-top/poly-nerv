package gemini

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
			name: "BeforeTool maps to PreToolUse",
			input: map[string]any{
				"hook_event_name": "BeforeTool",
				"session_id":      "sess-1",
				"cwd":             "/tmp",
				"tool_name":       "Shell",
			},
			wantEvent: "PreToolUse",
		},
		{
			name: "AfterTool maps to PostToolUse",
			input: map[string]any{
				"hook_event_name": "AfterTool",
				"session_id":      "sess-2",
			},
			wantEvent: "PostToolUse",
		},
		{
			name: "SessionStart maps to SessionStart",
			input: map[string]any{
				"hook_event_name": "SessionStart",
			},
			wantEvent: "SessionStart",
		},
		{
			name: "BeforeAgent maps to SubagentStart",
			input: map[string]any{
				"hook_event_name": "BeforeAgent",
			},
			wantEvent: "SubagentStart",
		},
		{
			name: "PreCompress maps to PreCompact",
			input: map[string]any{
				"hook_event_name": "PreCompress",
			},
			wantEvent: "PreCompact",
		},
		{
			name:    "missing event name",
			input:   map[string]any{},
			wantErr: true,
		},
		{
			name: "unknown event",
			input: map[string]any{
				"hook_event_name": "NoSuchEvent",
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
			if env.CLI.Name != "gemini" {
				t.Errorf("cli.name = %q, want %q", env.CLI.Name, "gemini")
			}
		})
	}
}

func TestToEnvelope_SessionInfo(t *testing.T) {
	env, err := ToEnvelope(map[string]any{
		"hook_event_name": "SessionStart",
		"session_id":      "abc-123",
		"cwd":             "/home/user/project",
		"timestamp":       "2026-04-11T10:00:00Z",
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
	if m["decision"] != "deny" {
		t.Errorf("decision = %v, want deny", m["decision"])
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
		t.Errorf("decision = %v, want allow (warn→allow)", m["decision"])
	}
	if m["systemMessage"] != "caution" {
		t.Errorf("systemMessage = %v, want caution", m["systemMessage"])
	}
}

func TestFromDecision_Rewrite(t *testing.T) {
	rewrite := map[string]any{"command": "ls -la"}
	m, code, err := FromDecision(merger.Decision{
		Action:  merger.Rewrite,
		Rewrite: rewrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	hso, ok := m["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatal("missing hookSpecificOutput")
	}
	if hso["tool_input"] == nil {
		t.Error("missing tool_input in hookSpecificOutput")
	}
}

func TestTranslateEventName(t *testing.T) {
	if got := TranslateEventName("BeforeTool"); got != "PreToolUse" {
		t.Errorf("got %q, want PreToolUse", got)
	}
	if got := TranslateEventName("Unknown"); got != "Unknown" {
		t.Errorf("got %q, want Unknown passthrough", got)
	}
}

func TestReverseEventName(t *testing.T) {
	if got := ReverseEventName(events.EventPreToolUse); got != "BeforeTool" {
		t.Errorf("got %q, want BeforeTool", got)
	}
	if got := ReverseEventName(events.EventTeammateIdle); got != "" {
		t.Errorf("got %q, want empty (unsupported)", got)
	}
}
