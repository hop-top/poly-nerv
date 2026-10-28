package opencode

import (
	"encoding/json"
	"testing"

	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

func TestTranslateEventName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{OCSessionCreated, "SessionStart"},
		{OCSessionDeleted, "SessionEnd"},
		{OCSessionIdle, "Stop"},
		{OCSessionError, "StopFailure"},
		{OCSessionCompacted, "PostCompact"},
		{OCToolExecBefore, "PreToolUse"},
		{OCToolExecAfter, "PostToolUse"},
		{OCFileEdited, "FileChanged"},
		{OCPermissionAsked, "PermissionRequest"},
		{OCPermissionReplied, "PermissionDenied"},
		{OCTUIPromptAppend, "UserPromptSubmit"},
		{OCTodoUpdated, "TaskCompleted"},
		{"unknown.event", ""},
	}

	for _, tt := range tests {
		got := TranslateEventName(tt.input)
		if got != tt.want {
			t.Errorf("TranslateEventName(%q) = %q, want %q",
				tt.input, got, tt.want)
		}
	}
}

func TestReverseEventName(t *testing.T) {
	tests := []struct {
		input events.Event
		want  string
	}{
		{events.EventSessionStart, OCSessionCreated},
		{events.EventPreToolUse, OCToolExecBefore},
		{events.EventPostToolUse, OCToolExecAfter},
		{events.EventTeammateIdle, ""}, // unsupported
	}

	for _, tt := range tests {
		got := ReverseEventName(tt.input)
		if got != tt.want {
			t.Errorf("ReverseEventName(%q) = %q, want %q",
				tt.input, got, tt.want)
		}
	}
}

func TestToEnvelope_Native(t *testing.T) {
	raw := `{"type":"session.created","session_id":"abc123"}`
	env, err := ToEnvelope([]byte(raw), "fallback-sid")
	if err != nil {
		t.Fatalf("ToEnvelope() error: %v", err)
	}
	if env.Event != "SessionStart" {
		t.Errorf("Event = %q, want SessionStart", env.Event)
	}
	if env.Session.ID != "abc123" {
		t.Errorf("Session.ID = %q, want abc123", env.Session.ID)
	}
	if env.CLI.Name != "opencode" {
		t.Errorf("CLI.Name = %q, want opencode", env.CLI.Name)
	}
}

func TestToEnvelope_FallbackSessionID(t *testing.T) {
	raw := `{"type":"session.created"}`
	env, err := ToEnvelope([]byte(raw), "fallback-sid")
	if err != nil {
		t.Fatalf("ToEnvelope() error: %v", err)
	}
	if env.Session.ID != "fallback-sid" {
		t.Errorf("Session.ID = %q, want fallback-sid", env.Session.ID)
	}
}

func TestToEnvelope_ToolExecAfterError(t *testing.T) {
	raw := `{"type":"tool.execute.after","tool":"bash","error":"exit 1"}`
	env, err := ToEnvelope([]byte(raw), "sid")
	if err != nil {
		t.Fatalf("ToEnvelope() error: %v", err)
	}
	if env.Event != "PostToolUseFailure" {
		t.Errorf("Event = %q, want PostToolUseFailure", env.Event)
	}
}

func TestToEnvelope_PermissionReplyAllow(t *testing.T) {
	raw := `{"type":"permission.replied","decision":"allow"}`
	_, err := ToEnvelope([]byte(raw), "sid")
	if err == nil {
		t.Error("expected error for non-deny permission.replied")
	}
}

func TestToEnvelope_PermissionReplyDeny(t *testing.T) {
	raw := `{"type":"permission.replied","decision":"deny"}`
	env, err := ToEnvelope([]byte(raw), "sid")
	if err != nil {
		t.Fatalf("ToEnvelope() error: %v", err)
	}
	if env.Event != "PermissionDenied" {
		t.Errorf("Event = %q, want PermissionDenied", env.Event)
	}
}

func TestToEnvelope_Unknown(t *testing.T) {
	raw := `{"type":"bogus.event"}`
	_, err := ToEnvelope([]byte(raw), "sid")
	if err == nil {
		t.Error("expected error for unknown event")
	}
}

func TestFromDecision_Allow(t *testing.T) {
	d := merger.Decision{Action: merger.Allow}
	out, err := FromDecision(d)
	if err != nil {
		t.Fatalf("FromDecision() error: %v", err)
	}
	var dj DecisionJSON
	if err := json.Unmarshal(out, &dj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dj.Action != "allow" {
		t.Errorf("Action = %q, want allow", dj.Action)
	}
}

func TestFromDecision_Block(t *testing.T) {
	d := merger.Decision{Action: merger.Block, Message: "denied"}
	out, err := FromDecision(d)
	if err != nil {
		t.Fatalf("FromDecision() error: %v", err)
	}
	var dj DecisionJSON
	if err := json.Unmarshal(out, &dj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dj.Action != "block" {
		t.Errorf("Action = %q, want block", dj.Action)
	}
	if dj.Message != "denied" {
		t.Errorf("Message = %q, want denied", dj.Message)
	}
}

func TestFromDecision_Rewrite(t *testing.T) {
	d := merger.Decision{
		Action:  merger.Rewrite,
		Rewrite: map[string]string{"command": "ls -la"},
	}
	out, err := FromDecision(d)
	if err != nil {
		t.Fatalf("FromDecision() error: %v", err)
	}
	var dj DecisionJSON
	if err := json.Unmarshal(out, &dj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dj.Action != "rewrite" {
		t.Errorf("Action = %q, want rewrite", dj.Action)
	}
	if dj.Rewrite == nil {
		t.Error("Rewrite is nil, want non-nil")
	}
}
