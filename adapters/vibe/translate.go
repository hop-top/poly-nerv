// Package vibe implements the nerv adapter for Mistral's Vibe CLI.
package vibe

import (
	"encoding/json"
	"fmt"
	"time"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// vibeToNerv maps Vibe native event names to nerv canonical names.
// Source: vibe/core/types.py
var vibeToNerv = map[string]events.Event{
	// Native events
	"UserMessageEvent":         events.EventUserPromptSubmit,
	"AssistantEvent":           events.EventStop,
	"ToolCallEvent":            events.EventPreToolUse,
	"ToolResultEvent":          events.EventPostToolUse,
	"WaitingForInputEvent":     events.EventElicitation,
	"CompactStartEvent":        events.EventPreCompact,
	"CompactEndEvent":          events.EventPostCompact,
	"AgentProfileChangedEvent": events.EventConfigChange,
	// Callbacks (synthesized via middleware)
	"ApprovalCallback":    events.EventPermissionRequest,
	"SwitchAgentCallback": events.EventSubagentStart,
}

// nervToVibe maps nerv canonical names to Vibe native event names.
var nervToVibe = map[events.Event]string{
	events.EventUserPromptSubmit:  "UserMessageEvent",
	events.EventStop:              "AssistantEvent",
	events.EventPreToolUse:        "ToolCallEvent",
	events.EventPostToolUse:       "ToolResultEvent",
	events.EventElicitation:       "WaitingForInputEvent",
	events.EventPreCompact:        "CompactStartEvent",
	events.EventPostCompact:       "CompactEndEvent",
	events.EventConfigChange:      "AgentProfileChangedEvent",
	events.EventPermissionRequest: "ApprovalCallback",
	events.EventSubagentStart:     "SwitchAgentCallback",
}

// nervActionToVibe maps nerv merger actions to Vibe decision strings.
var nervActionToVibe = map[merger.Action]string{
	merger.Allow:   "allow",
	merger.Warn:    "allow",
	merger.Block:   "block",
	merger.Rewrite: "allow",
}

// ToEnvelope converts a Vibe CLI event (parsed JSON map) into a nerv
// Envelope. Returns an error if the event name is unrecognized or the
// payload is malformed.
func ToEnvelope(vibeEvent map[string]any) (*envelope.Envelope, error) {
	rawName, ok := vibeEvent["event"].(string)
	if !ok || rawName == "" {
		return nil, fmt.Errorf("vibe: missing event field")
	}

	nervEvent, known := vibeToNerv[rawName]
	if !known {
		return nil, fmt.Errorf("vibe: unknown event %q", rawName)
	}

	// Extract session info.
	var sess envelope.SessionInfo
	if sid, ok := vibeEvent["session_id"].(string); ok {
		sess.ID = sid
	}
	if cwd, ok := vibeEvent["cwd"].(string); ok {
		sess.CWD = cwd
	}

	// Extract timestamp.
	ts := time.Now()
	if raw, ok := vibeEvent["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			ts = parsed
		}
	}

	// Marshal the full Vibe event as the payload.
	payload, err := json.Marshal(vibeEvent)
	if err != nil {
		return nil, fmt.Errorf("vibe: marshal payload: %w", err)
	}

	env := &envelope.Envelope{
		NervVersion: "1.0-draft",
		Event:       string(nervEvent),
		Timestamp:   ts,
		CLI: envelope.CLIInfo{
			Name: "vibe",
		},
		Session: sess,
		Payload: payload,
	}

	return env, nil
}

// vibeDecision is the Vibe CLI stdout decision format.
type vibeDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

// FromDecision converts a nerv merger Decision into a Vibe-native
// decision map and exit code.
func FromDecision(d merger.Decision) (map[string]any, int, error) {
	vibeAction, ok := nervActionToVibe[d.Action]
	if !ok {
		return nil, 0, fmt.Errorf(
			"vibe: unsupported action %q", d.Action,
		)
	}

	exitCode := 0
	if d.Action == merger.Block {
		exitCode = 2
	}

	out := vibeDecision{
		Decision: vibeAction,
	}

	if d.Message != "" {
		out.Reason = d.Message
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return nil, 0, fmt.Errorf("vibe: marshal decision: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, 0, fmt.Errorf("vibe: unmarshal decision: %w", err)
	}

	return m, exitCode, nil
}

// TranslateEventName converts a Vibe event name to nerv canonical
// name. Returns the input unchanged if no mapping exists.
func TranslateEventName(vibeName string) string {
	if nerv, ok := vibeToNerv[vibeName]; ok {
		return string(nerv)
	}
	return vibeName
}

// ReverseEventName converts a nerv event name to a Vibe native name.
// Returns empty string if no reverse mapping exists.
func ReverseEventName(nervName events.Event) string {
	return nervToVibe[nervName]
}
