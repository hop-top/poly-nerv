// Package codex implements the nerv adapter for OpenAI Codex CLI.
//
// Codex has the most limited hook surface (3 native events). This adapter
// layers shell regex matching, fswatch daemons, and binary wrapping to
// synthesize up to 15 events from the 26-event nerv taxonomy.
package codex

import (
	"fmt"

	"hop.top/nerv/internal/events"
)

// codexEventMap maps Codex-native event names to nerv events.
var codexEventMap = map[string]events.Event{
	"SessionStart":     events.EventSessionStart,
	"UserPromptSubmit": events.EventUserPromptSubmit,
	"PreToolUse":       events.EventPreToolUse,
	"PostToolUse":      events.EventPostToolUse,
	"Stop":             events.EventStop,
}

// TranslateEvent converts a Codex event name to its nerv equivalent.
// Returns an error if the event name is unknown.
func TranslateEvent(codexEvent string) (events.Event, error) {
	e, ok := codexEventMap[codexEvent]
	if !ok {
		return "", fmt.Errorf("codex: unknown event %q", codexEvent)
	}
	return e, nil
}

// IsNativeEvent reports whether the event is natively supported by Codex
// (full parity only; excludes scope-limited events).
func IsNativeEvent(codexEvent string) bool {
	for _, e := range NativeEvents() {
		if e == codexEvent {
			return true
		}
	}
	return false
}

// IsNativeOrScopeLimited reports whether the event exists natively in
// Codex, including scope-limited events (PreToolUse/PostToolUse).
func IsNativeOrScopeLimited(codexEvent string) bool {
	_, ok := codexEventMap[codexEvent]
	return ok
}

// NativeEvents returns Codex-native event names with full parity.
func NativeEvents() []string {
	return []string{
		"SessionStart",
		"UserPromptSubmit",
		"Stop",
	}
}

// ScopeLimitedEvents returns events that exist natively but with
// reduced scope (shell + apply_patch only).
func ScopeLimitedEvents() []string {
	return []string{
		"PreToolUse",
		"PostToolUse",
	}
}
