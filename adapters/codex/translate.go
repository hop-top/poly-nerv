// Package codex implements the nerv adapter for OpenAI Codex CLI.
//
// Codex has the most limited hook surface (3 native events). This adapter
// layers shell regex matching, fswatch daemons, and binary wrapping to
// synthesize up to 15 events from the 26-event nerv taxonomy.
package codex

import (
	"encoding/json"
	"fmt"

	"hop.top/nerv/adapters/claude"
	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
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

// contextEvents are the Codex hook events whose stdout JSON accepts
// hookSpecificOutput.additionalContext. Codex Stop, SubagentStop and
// compaction outputs reject unknown fields, so context is dropped there
// even though Claude accepts it on Stop.
//
// Source: openai/codex codex-rs/hooks/schema/generated/*.command.output
// .schema.json (additionalContext present on session-start,
// user-prompt-submit, pre-tool-use, post-tool-use, subagent-start only)
// and codex-rs/hooks/src/events/*.rs (consumed as model input).
var contextEvents = map[string]bool{
	"SessionStart":     true,
	"UserPromptSubmit": true,
	"PreToolUse":       true,
	"PostToolUse":      true,
}

// FromEventDecision converts a merged decision into Codex's hook stdout
// JSON and exit code for the given native event name. Codex's hook
// output mirrors Claude Code's, so action encoding is shared with the
// claude translator; only the context channel set differs. Context is
// dropped on block and on events outside contextEvents.
func FromEventDecision(
	d merger.Decision, codexEvent string,
) ([]byte, int, error) {
	cd := claude.Decision{
		Action: string(d.Action),
		Reason: d.Message,
	}
	if d.Action != merger.Block && contextEvents[codexEvent] {
		cd.Context = d.Context
	}
	if d.Rewrite != nil {
		payload, err := json.Marshal(d.Rewrite)
		if err != nil {
			return nil, 0, fmt.Errorf("codex: marshal rewrite: %w", err)
		}
		cd.Payload = payload
	}
	return (&claude.Translator{}).FromDecision(cd, codexEvent)
}
