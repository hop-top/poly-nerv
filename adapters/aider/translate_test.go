package aider

import (
	"testing"

	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

func TestToEnvelope_SynthesizedEvents(t *testing.T) {
	tests := []struct {
		name      string
		input     map[string]any
		wantEvent string
		wantErr   bool
	}{
		{
			name: "PreToolUse from bridge",
			input: map[string]any{
				"hook_event_name": "PreToolUse",
				"session_id":      "sess-1",
				"cwd":             "/tmp",
				"tool_name":       "edit",
			},
			wantEvent: "PreToolUse",
		},
		{
			name: "PostToolUse from bridge",
			input: map[string]any{
				"hook_event_name": "PostToolUse",
				"session_id":      "sess-2",
			},
			wantEvent: "PostToolUse",
		},
		{
			name: "SessionStart from bridge",
			input: map[string]any{
				"hook_event_name": "SessionStart",
			},
			wantEvent: "SessionStart",
		},
		{
			name: "SessionEnd from bridge",
			input: map[string]any{
				"hook_event_name": "SessionEnd",
			},
			wantEvent: "SessionEnd",
		},
		{
			name: "FileChanged from bridge",
			input: map[string]any{
				"hook_event_name": "FileChanged",
				"session_id":      "sess-3",
				"file":            "main.py",
			},
			wantEvent: "FileChanged",
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
				t.Errorf("event = %q, want %q",
					env.Event, tt.wantEvent)
			}
			if env.CLI.Name != "aider" {
				t.Errorf("cli.name = %q, want %q",
					env.CLI.Name, "aider")
			}
			if !env.Synthesized {
				t.Error("expected Synthesized=true")
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
		t.Errorf("session.id = %q, want %q",
			env.Session.ID, "abc-123")
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
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
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
		t.Errorf("decision = %v, want allow (warn->allow)",
			m["decision"])
	}
}

func TestTranslateEventName(t *testing.T) {
	if got := TranslateEventName("PreToolUse"); got != "PreToolUse" {
		t.Errorf("got %q, want PreToolUse", got)
	}
	if got := TranslateEventName("FileChanged"); got != "FileChanged" {
		t.Errorf("got %q, want FileChanged", got)
	}
	if got := TranslateEventName("Unknown"); got != "Unknown" {
		t.Errorf("got %q, want Unknown passthrough", got)
	}
}

func TestReverseEventName(t *testing.T) {
	if got := ReverseEventName(events.EventPreToolUse); got != "PreToolUse" {
		t.Errorf("got %q, want PreToolUse", got)
	}
	if got := ReverseEventName(events.EventTeammateIdle); got != "" {
		t.Errorf("got %q, want empty (unsupported)", got)
	}
}
