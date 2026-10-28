// Package opencode implements the nerv adapter for OpenCode CLI.
//
// OpenCode uses JS/TS plugins with dotted event names (session.created,
// tool.execute.before). This package translates between OpenCode's event
// model and nerv's PascalCase envelope format.
package opencode

import (
	"encoding/json"
	"fmt"
	"time"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// OpenCode dotted event names.
const (
	OCSessionCreated    = "session.created"
	OCSessionDeleted    = "session.deleted"
	OCSessionIdle       = "session.idle"
	OCSessionError      = "session.error"
	OCSessionCompacted  = "session.compacted"
	OCToolExecBefore    = "tool.execute.before"
	OCToolExecAfter     = "tool.execute.after"
	OCFileEdited        = "file.edited"
	OCPermissionAsked   = "permission.asked"
	OCPermissionReplied = "permission.replied"
	OCTUIPromptAppend   = "tui.prompt.append"
	OCTodoUpdated       = "todo.updated"
)

// nativeToNerv maps OpenCode dotted event names to nerv events.
var nativeToNerv = map[string]events.Event{
	OCSessionCreated:    events.EventSessionStart,
	OCSessionDeleted:    events.EventSessionEnd,
	OCSessionIdle:       events.EventStop,
	OCSessionError:      events.EventStopFailure,
	OCSessionCompacted:  events.EventPostCompact,
	OCToolExecBefore:    events.EventPreToolUse,
	OCToolExecAfter:     events.EventPostToolUse,
	OCFileEdited:        events.EventFileChanged,
	OCPermissionAsked:   events.EventPermissionRequest,
	OCPermissionReplied: events.EventPermissionDenied,
	OCTUIPromptAppend:   events.EventUserPromptSubmit,
	OCTodoUpdated:       events.EventTaskCompleted,
}

// nervToNative is the reverse mapping for outbound translation.
var nervToNative map[events.Event]string

func init() {
	nervToNative = make(map[events.Event]string, len(nativeToNerv))
	for oc, nerv := range nativeToNerv {
		nervToNative[nerv] = oc
	}
}

// OpenCodeEvent is the wire format OpenCode sends to plugin handlers.
type OpenCodeEvent struct {
	Type      string          `json:"type"`
	Tool      string          `json:"tool,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
	Error     string          `json:"error,omitempty"`
	Path      string          `json:"path,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Decision  string          `json:"decision,omitempty"`
}

// ToEnvelope translates an OpenCode plugin event into a nerv envelope.
// Returns error if the event type is unknown.
func ToEnvelope(raw []byte, sessionID string) (*envelope.Envelope, error) {
	var oe OpenCodeEvent
	if err := json.Unmarshal(raw, &oe); err != nil {
		return nil, fmt.Errorf("opencode: unmarshal event: %w", err)
	}

	nervEvent, ok := nativeToNerv[oe.Type]
	if !ok {
		return nil, fmt.Errorf("opencode: unknown event %q", oe.Type)
	}

	// Handle PostToolUseFailure: tool.execute.after with error field.
	if oe.Type == OCToolExecAfter && oe.Error != "" {
		nervEvent = events.EventPostToolUseFailure
	}

	// Handle PermissionDenied filter: only map if decision is deny.
	if oe.Type == OCPermissionReplied && oe.Decision != "deny" {
		// Non-deny permission replies are not mapped.
		return nil, fmt.Errorf(
			"opencode: permission.replied with decision=%q skipped",
			oe.Decision,
		)
	}

	sid := sessionID
	if oe.SessionID != "" {
		sid = oe.SessionID
	}

	synthesized := false
	var synthSource *string

	env := &envelope.Envelope{
		NervVersion: "1.0",
		Event:       string(nervEvent),
		Timestamp:   time.Now().UTC(),
		CLI: envelope.CLIInfo{
			Name: "opencode",
		},
		Session: envelope.SessionInfo{
			ID: sid,
		},
		Payload:         marshalPayload(oe),
		Synthesized:     synthesized,
		SynthesisSource: synthSource,
	}

	return env, nil
}

// DecisionJSON is the wire format the TS shim expects back from nervd.
type DecisionJSON struct {
	Action  string          `json:"action"`
	Message string          `json:"message,omitempty"`
	Rewrite json.RawMessage `json:"rewrite,omitempty"`
}

// FromDecision converts a nerv merger.Decision into the JSON the
// TypeScript shim will read. For blocking events, the shim uses
// the action to decide whether to throw (block) or pass through.
func FromDecision(d merger.Decision) ([]byte, error) {
	dj := DecisionJSON{
		Action:  string(d.Action),
		Message: d.Message,
	}

	if d.Rewrite != nil {
		rw, err := json.Marshal(d.Rewrite)
		if err != nil {
			return nil, fmt.Errorf("opencode: marshal rewrite: %w", err)
		}
		dj.Rewrite = rw
	}

	return json.Marshal(dj)
}

// marshalPayload serializes the full OpenCode event as JSON payload.
// Includes tool, output, error, path, decision fields for downstream hooks.
func marshalPayload(oe OpenCodeEvent) json.RawMessage {
	b, err := json.Marshal(oe)
	if err != nil {
		return oe.Input // fallback to original input
	}
	return b
}

// TranslateEventName converts an OpenCode dotted event name to its
// nerv PascalCase equivalent. Returns empty string if unknown.
func TranslateEventName(ocEvent string) string {
	e, ok := nativeToNerv[ocEvent]
	if !ok {
		return ""
	}
	return string(e)
}

// ReverseEventName converts a nerv PascalCase event to its OpenCode
// dotted equivalent. Returns empty string if unknown.
func ReverseEventName(nervEvent events.Event) string {
	return nervToNative[nervEvent]
}
