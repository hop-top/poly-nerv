package codex

import (
	"testing"

	"hop.top/nerv/internal/events"
)

func TestTranslateEvent_Native(t *testing.T) {
	tests := []struct {
		input string
		want  events.Event
	}{
		{"SessionStart", events.EventSessionStart},
		{"UserPromptSubmit", events.EventUserPromptSubmit},
		{"Stop", events.EventStop},
		{"PreToolUse", events.EventPreToolUse},
		{"PostToolUse", events.EventPostToolUse},
	}
	for _, tt := range tests {
		got, err := TranslateEvent(tt.input)
		if err != nil {
			t.Errorf("TranslateEvent(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("TranslateEvent(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTranslateEvent_Unknown(t *testing.T) {
	_, err := TranslateEvent("NonExistentEvent")
	if err == nil {
		t.Error("TranslateEvent(unknown) should return error")
	}
}

func TestIsNativeEvent(t *testing.T) {
	if !IsNativeEvent("SessionStart") {
		t.Error("SessionStart should be native")
	}
	if IsNativeEvent("FileChanged") {
		t.Error("FileChanged should not be native")
	}
}

func TestNativeEvents(t *testing.T) {
	got := NativeEvents()
	if len(got) != 3 {
		t.Errorf("NativeEvents() returned %d events, want 3", len(got))
	}
}

func TestScopeLimitedEvents(t *testing.T) {
	got := ScopeLimitedEvents()
	if len(got) != 2 {
		t.Errorf("ScopeLimitedEvents() returned %d events, want 2", len(got))
	}
}
