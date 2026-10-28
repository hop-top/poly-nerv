// Package aider implements the nerv adapter for Aider CLI.
package aider

import (
	"encoding/json"
	"fmt"
	"time"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// aiderToNerv maps Aider synthesized event names to nerv canonical names.
// Aider has no native hook events; all events are synthesized by the
// nerv bridge script that runs via lint-cmd/test-cmd hooks.
var aiderToNerv = map[string]events.Event{
	"SessionStart": events.EventSessionStart,
	"SessionEnd":   events.EventSessionEnd,
	"PreToolUse":   events.EventPreToolUse,
	"PostToolUse":  events.EventPostToolUse,
	"FileChanged":  events.EventFileChanged,
}

// nervToAider maps nerv canonical names to Aider event names.
var nervToAider = map[events.Event]string{
	events.EventSessionStart: "SessionStart",
	events.EventSessionEnd:   "SessionEnd",
	events.EventPreToolUse:   "PreToolUse",
	events.EventPostToolUse:  "PostToolUse",
	events.EventFileChanged:  "FileChanged",
}

// nervActionToAider maps nerv merger actions to Aider decision strings.
var nervActionToAider = map[merger.Action]string{
	merger.Allow:   "allow",
	merger.Warn:    "allow",
	merger.Block:   "block",
	merger.Rewrite: "allow",
}

// ToEnvelope converts an Aider bridge event (parsed JSON map) into a
// nerv Envelope.
func ToEnvelope(aiderEvent map[string]any) (*envelope.Envelope, error) {
	rawName, ok := aiderEvent["hook_event_name"].(string)
	if !ok || rawName == "" {
		return nil, fmt.Errorf("aider: missing hook_event_name")
	}

	nervEvent, known := aiderToNerv[rawName]
	if !known {
		return nil, fmt.Errorf("aider: unknown event %q", rawName)
	}

	var sess envelope.SessionInfo
	if sid, ok := aiderEvent["session_id"].(string); ok {
		sess.ID = sid
	}
	if cwd, ok := aiderEvent["cwd"].(string); ok {
		sess.CWD = cwd
	}

	ts := time.Now()
	if raw, ok := aiderEvent["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			ts = parsed
		}
	}

	payload, err := json.Marshal(aiderEvent)
	if err != nil {
		return nil, fmt.Errorf("aider: marshal payload: %w", err)
	}

	env := &envelope.Envelope{
		NervVersion: "1.0-draft",
		Event:       string(nervEvent),
		Timestamp:   ts,
		Synthesized: true,
		CLI: envelope.CLIInfo{
			Name: "aider",
		},
		Session: sess,
		Payload: payload,
	}

	return env, nil
}

// aiderDecision is the Aider bridge stdout decision format.
type aiderDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

// FromDecision converts a nerv merger Decision into an Aider-native
// decision map and exit code.
func FromDecision(d merger.Decision) (map[string]any, int, error) {
	aiderAction, ok := nervActionToAider[d.Action]
	if !ok {
		return nil, 0, fmt.Errorf(
			"aider: unsupported action %q", d.Action,
		)
	}

	exitCode := 0
	if d.Action == merger.Block {
		exitCode = 1
	}

	out := aiderDecision{
		Decision: aiderAction,
	}
	if d.Message != "" {
		out.Reason = d.Message
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return nil, 0, fmt.Errorf("aider: marshal decision: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, 0, fmt.Errorf("aider: unmarshal decision: %w", err)
	}

	return m, exitCode, nil
}

// TranslateEventName converts an Aider event name to nerv canonical
// name. Returns the input unchanged if no mapping exists.
func TranslateEventName(aiderName string) string {
	if nerv, ok := aiderToNerv[aiderName]; ok {
		return string(nerv)
	}
	return aiderName
}

// ReverseEventName converts a nerv event name to an Aider event name.
// Returns empty string if no reverse mapping exists.
func ReverseEventName(nervName events.Event) string {
	return nervToAider[nervName]
}
