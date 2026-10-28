package goose

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
		wantSynth bool
		wantErr   bool
	}{
		{
			name: "session.start maps to SessionStart (native)",
			input: map[string]any{
				"type":       "session.start",
				"session_id": "sess-1",
			},
			wantEvent: "SessionStart",
			wantSynth: false,
		},
		{
			name: "session.end maps to SessionEnd (native)",
			input: map[string]any{
				"type":       "session.end",
				"session_id": "sess-2",
			},
			wantEvent: "SessionEnd",
			wantSynth: false,
		},
		{
			name: "finish maps to Stop (native)",
			input: map[string]any{
				"type":       "finish",
				"session_id": "sess-f",
				"reason":     "complete",
			},
			wantEvent: "Stop",
			wantSynth: false,
		},
		{
			name: "notification maps to Notification (native)",
			input: map[string]any{
				"type":       "notification",
				"request_id": "req-1",
			},
			wantEvent: "Notification",
			wantSynth: false,
		},
		{
			name: "update_conversation maps to PostCompact (native)",
			input: map[string]any{
				"type": "update_conversation",
			},
			wantEvent: "PostCompact",
			wantSynth: false,
		},
		{
			name: "action_required maps to PermissionRequest (native)",
			input: map[string]any{
				"type":      "action_required",
				"tool_name": "shell",
			},
			wantEvent: "PermissionRequest",
			wantSynth: false,
		},
		{
			name: "permission_denied maps to PermissionDenied (native)",
			input: map[string]any{
				"type": "permission_denied",
			},
			wantEvent: "PermissionDenied",
			wantSynth: false,
		},
		{
			name: "subagent_created maps to SubagentStart (native)",
			input: map[string]any{
				"type":    "subagent_created",
				"task_id": "task-1",
			},
			wantEvent: "SubagentStart",
			wantSynth: false,
		},
		{
			name: "subagent_completed maps to SubagentStop (native)",
			input: map[string]any{
				"type":    "subagent_completed",
				"task_id": "task-1",
			},
			wantEvent: "SubagentStop",
			wantSynth: false,
		},
		{
			name: "subagent_terminated maps to SubagentStop (native)",
			input: map[string]any{
				"type":    "subagent_terminated",
				"task_id": "task-2",
			},
			wantEvent: "SubagentStop",
			wantSynth: false,
		},
		{
			name: "tasks_complete maps to TaskCompleted (native)",
			input: map[string]any{
				"type": "tasks_complete",
			},
			wantEvent: "TaskCompleted",
			wantSynth: false,
		},
		{
			name: "error maps to StopFailure (native)",
			input: map[string]any{
				"type":  "error",
				"error": "API timeout",
			},
			wantEvent: "StopFailure",
			wantSynth: false,
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
			if env.CLI.Name != "goose" {
				t.Errorf("cli.name = %q, want %q",
					env.CLI.Name, "goose")
			}
			if env.Synthesized != tt.wantSynth {
				t.Errorf("synthesized = %v, want %v",
					env.Synthesized, tt.wantSynth)
			}
		})
	}
}

func TestToEnvelope_SynthesizedEvents(t *testing.T) {
	tests := []struct {
		name       string
		input      map[string]any
		wantEvent  string
		wantSource string
	}{
		{
			name: "tool.before maps to PreToolUse (synthesized)",
			input: map[string]any{
				"type":       "tool.before",
				"session_id": "sess-3",
				"tool_name":  "shell",
			},
			wantEvent:  "PreToolUse",
			wantSource: "extension",
		},
		{
			name: "tool.after maps to PostToolUse (synthesized)",
			input: map[string]any{
				"type":       "tool.after",
				"session_id": "sess-4",
			},
			wantEvent:  "PostToolUse",
			wantSource: "extension",
		},
		{
			name: "tool.error maps to PostToolUseFailure (synthesized)",
			input: map[string]any{
				"type":  "tool.error",
				"error": "timeout",
			},
			wantEvent:  "PostToolUseFailure",
			wantSource: "extension",
		},
		{
			name: "file.changed maps to FileChanged (synthesized)",
			input: map[string]any{
				"type": "file.changed",
				"file": "main.rs",
			},
			wantEvent:  "FileChanged",
			wantSource: "extension",
		},
		{
			name: "cwd.changed maps to CwdChanged (synthesized)",
			input: map[string]any{
				"type": "cwd.changed",
				"cwd":  "/new/dir",
			},
			wantEvent:  "CwdChanged",
			wantSource: "shell-match",
		},
		{
			name: "worktree.created maps to WorktreeCreate (synthesized)",
			input: map[string]any{
				"type": "worktree.created",
				"path": "/tmp/wt",
			},
			wantEvent:  "WorktreeCreate",
			wantSource: "shell-match",
		},
		{
			name: "worktree.removed maps to WorktreeRemove (synthesized)",
			input: map[string]any{
				"type": "worktree.removed",
				"path": "/tmp/wt",
			},
			wantEvent:  "WorktreeRemove",
			wantSource: "shell-match",
		},
		{
			name: "compaction.request maps to PreCompact (synthesized)",
			input: map[string]any{
				"type": "compaction.request",
			},
			wantEvent:  "PreCompact",
			wantSource: "signal",
		},
		{
			name: "instructions.loaded maps to InstructionsLoaded (synthesized)",
			input: map[string]any{
				"type": "instructions.loaded",
				"file": "goosehints",
			},
			wantEvent:  "InstructionsLoaded",
			wantSource: "extension",
		},
		{
			name: "request.start maps to UserPromptSubmit (synthesized)",
			input: map[string]any{
				"type": "request.start",
			},
			wantEvent:  "UserPromptSubmit",
			wantSource: "sse",
		},
		{
			name: "tasks_update maps to TaskCreated (synthesized via notification)",
			input: map[string]any{
				"type":   "tasks_update",
				"status": "Running",
			},
			wantEvent:  "TaskCreated",
			wantSource: "notification",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := ToEnvelope(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if env.Event != tt.wantEvent {
				t.Errorf("event = %q, want %q",
					env.Event, tt.wantEvent)
			}
			if !env.Synthesized {
				t.Error("expected synthesized = true")
			}
			if env.SynthesisSource == nil {
				t.Fatal("expected non-nil synthesis_source")
			}
			if *env.SynthesisSource != tt.wantSource {
				t.Errorf("synthesis_source = %q, want %q",
					*env.SynthesisSource, tt.wantSource)
			}
		})
	}
}

func TestToEnvelope_ShellSynthesis(t *testing.T) {
	tests := []struct {
		name      string
		input     map[string]any
		wantEvent string
	}{
		{
			name: "shell cd synthesizes CwdChanged",
			input: map[string]any{
				"type":      "tool.after",
				"tool_name": "developer__shell",
				"tool_input": map[string]any{
					"command": "cd /tmp/project",
				},
			},
			wantEvent: "CwdChanged",
		},
		{
			name: "shell worktree add synthesizes WorktreeCreate",
			input: map[string]any{
				"type":      "tool.after",
				"tool_name": "developer__shell",
				"tool_input": map[string]any{
					"command": "git worktree add /tmp/wt feat",
				},
			},
			wantEvent: "WorktreeCreate",
		},
		{
			name: "shell git hop add synthesizes WorktreeCreate",
			input: map[string]any{
				"type":      "tool.after",
				"tool_name": "developer__shell",
				"tool_input": map[string]any{
					"command": "git hop add feat/new",
				},
			},
			wantEvent: "WorktreeCreate",
		},
		{
			name: "shell worktree remove synthesizes WorktreeRemove",
			input: map[string]any{
				"type":      "tool.after",
				"tool_name": "developer__shell",
				"tool_input": map[string]any{
					"command": "git worktree remove /tmp/wt",
				},
			},
			wantEvent: "WorktreeRemove",
		},
		{
			name: "shell git hop remove synthesizes WorktreeRemove",
			input: map[string]any{
				"type":      "tool.after",
				"tool_name": "developer__shell",
				"tool_input": map[string]any{
					"command": "git hop remove feat/old",
				},
			},
			wantEvent: "WorktreeRemove",
		},
		{
			name: "text_editor synthesizes FileChanged",
			input: map[string]any{
				"type":      "tool.after",
				"tool_name": "developer__text_editor",
			},
			wantEvent: "FileChanged",
		},
		{
			name: "write_file synthesizes FileChanged",
			input: map[string]any{
				"type":      "tool.after",
				"tool_name": "developer__write_file",
			},
			wantEvent: "FileChanged",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, err := ToEnvelope(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if env.Event != tt.wantEvent {
				t.Errorf("event = %q, want %q",
					env.Event, tt.wantEvent)
			}
			if !env.Synthesized {
				t.Error("expected synthesized = true")
			}
		})
	}
}

func TestToEnvelope_Errors(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{
			name:  "missing type",
			input: map[string]any{},
		},
		{
			name: "unknown event",
			input: map[string]any{
				"type": "no.such.event",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ToEnvelope(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestToEnvelope_SessionInfo(t *testing.T) {
	env, err := ToEnvelope(map[string]any{
		"type":       "session.start",
		"session_id": "abc-123",
		"cwd":        "/home/user/project",
		"timestamp":  "2026-04-11T10:00:00Z",
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

func TestToEnvelope_FallbackEventName(t *testing.T) {
	env, err := ToEnvelope(map[string]any{
		"hook_event_name": "session.start",
		"session_id":      "fb-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if env.Event != "SessionStart" {
		t.Errorf("event = %q, want SessionStart", env.Event)
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
	if m["action"] != "continue" {
		t.Errorf("action = %v, want continue", m["action"])
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
	if m["action"] != "cancel" {
		t.Errorf("action = %v, want cancel", m["action"])
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
	if m["action"] != "continue" {
		t.Errorf("action = %v, want continue (warn->continue)",
			m["action"])
	}
	if m["message"] != "caution" {
		t.Errorf("message = %v, want caution", m["message"])
	}
}

func TestFromDecision_Rewrite(t *testing.T) {
	m, code, err := FromDecision(merger.Decision{
		Action:  merger.Rewrite,
		Message: "rewritten",
	})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if m["action"] != "continue" {
		t.Errorf("action = %v, want continue", m["action"])
	}
}

func TestTranslateEventName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"tool.before", "PreToolUse"},
		{"session.start", "SessionStart"},
		{"finish", "Stop"},
		{"notification", "Notification"},
		{"action_required", "PermissionRequest"},
		{"subagent_created", "SubagentStart"},
		{"tasks_complete", "TaskCompleted"},
		{"error", "StopFailure"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := TranslateEventName(tt.input); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReverseEventName(t *testing.T) {
	tests := []struct {
		input events.Event
		want  string
	}{
		{events.EventPreToolUse, "tool.before"},
		{events.EventStop, "finish"},
		{events.EventNotification, "notification"},
		{events.EventPermissionRequest, "action_required"},
		{events.EventSubagentStart, "subagent_created"},
		{events.EventTaskCompleted, "tasks_complete"},
		{events.EventStopFailure, "error"},
		{events.EventTeammateIdle, ""},
	}
	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			if got := ReverseEventName(tt.input); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
