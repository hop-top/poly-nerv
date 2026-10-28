package openhands

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
			name: "PreToolUse via hook_event_name",
			input: map[string]any{
				"hook_event_name": "PreToolUse",
				"session_id":      "sess-1",
				"cwd":             "/tmp",
				"tool_name":       "Bash",
			},
			wantEvent: "PreToolUse",
		},
		{
			name: "PostToolUse via hook_event_name",
			input: map[string]any{
				"hook_event_name": "PostToolUse",
				"session_id":      "sess-2",
			},
			wantEvent: "PostToolUse",
		},
		{
			name: "UserPromptSubmit via hook_event_name",
			input: map[string]any{
				"hook_event_name": "UserPromptSubmit",
				"session_id":      "sess-3",
			},
			wantEvent: "UserPromptSubmit",
		},
		{
			name: "SessionStart via hook_event_name",
			input: map[string]any{
				"hook_event_name": "SessionStart",
			},
			wantEvent: "SessionStart",
		},
		{
			name: "SessionEnd via hook_event_name",
			input: map[string]any{
				"hook_event_name": "SessionEnd",
			},
			wantEvent: "SessionEnd",
		},
		{
			name: "Stop via hook_event_name",
			input: map[string]any{
				"hook_event_name": "Stop",
			},
			wantEvent: "Stop",
		},
		{
			name: "fallback to event field",
			input: map[string]any{
				"event":      "PreToolUse",
				"session_id": "sess-fb",
			},
			wantEvent: "PreToolUse",
		},
		{
			name:    "missing both event fields",
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
				t.Errorf(
					"event = %q, want %q",
					env.Event, tt.wantEvent,
				)
			}
			if env.CLI.Name != "openhands" {
				t.Errorf(
					"cli.name = %q, want %q",
					env.CLI.Name, "openhands",
				)
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
		t.Errorf(
			"session.id = %q, want %q",
			env.Session.ID, "abc-123",
		)
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
	if m["action"] != "allow" {
		t.Errorf("action = %v, want allow", m["action"])
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
	if m["action"] != "block" {
		t.Errorf("action = %v, want block", m["action"])
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
	if m["action"] != "allow" {
		t.Errorf(
			"action = %v, want allow (warn->allow)",
			m["action"],
		)
	}
}

func TestTranslateEventName(t *testing.T) {
	if got := TranslateEventName("PreToolUse"); got != "PreToolUse" {
		t.Errorf("got %q, want PreToolUse", got)
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

func TestSynthesizeFromEventStream_SubagentStart(t *testing.T) {
	synth := SynthesizeFromEventStream(map[string]any{
		"action": "AgentDelegateAction",
		"agent":  "sub-agent-1",
	})
	if !hasSynthEvent(synth, events.EventSubagentStart) {
		t.Error("expected SubagentStart synthesis")
	}
}

func TestSynthesizeFromEventStream_FileChanged(t *testing.T) {
	synth := SynthesizeFromEventStream(map[string]any{
		"observation": "FileWrite",
		"path":        "main.go",
	})
	if !hasSynthEvent(synth, events.EventFileChanged) {
		t.Error("expected FileChanged synthesis")
	}
}

func TestSynthesizeFromEventStream_CwdChanged(t *testing.T) {
	synth := SynthesizeFromEventStream(map[string]any{
		"action":  "CmdRunAction",
		"command": "cd /tmp/other",
	})
	if !hasSynthEvent(synth, events.EventCwdChanged) {
		t.Error("expected CwdChanged synthesis")
	}
}

func TestSynthesizeFromEventStream_PostToolUseFailure(t *testing.T) {
	synth := SynthesizeFromEventStream(map[string]any{
		"observation": "ErrorObservation",
		"error":       "non-zero exit",
	})
	if !hasSynthEvent(synth, events.EventPostToolUseFailure) {
		t.Error("expected PostToolUseFailure synthesis")
	}
}

func TestSynthesizeFromEventStream_PermissionDenied(t *testing.T) {
	synth := SynthesizeFromEventStream(map[string]any{
		"observation": "UserRejectObservation",
	})
	if !hasSynthEvent(synth, events.EventPermissionDenied) {
		t.Error("expected PermissionDenied synthesis")
	}
}

func hasSynthEvent(synth []SyntheticEvent, want events.Event) bool {
	for _, s := range synth {
		if s.Event == want {
			return true
		}
	}
	return false
}
