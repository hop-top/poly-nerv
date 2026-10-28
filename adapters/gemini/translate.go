// Package gemini implements the nerv adapter for Gemini CLI.
package gemini

import (
	"encoding/json"
	"fmt"
	"time"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// geminiToNerv maps Gemini native event names to nerv canonical names.
var geminiToNerv = map[string]events.Event{
	"SessionStart": events.EventSessionStart,
	"SessionEnd":   events.EventSessionEnd,
	"BeforeTool":   events.EventPreToolUse,
	"AfterTool":    events.EventPostToolUse,
	"BeforeAgent":  events.EventSubagentStart,
	"AfterAgent":   events.EventSubagentStop,
	"BeforeModel":  events.EventUserPromptSubmit,
	// AfterModel maps to PostCompact only when compaction marker present.
	// Without marker detection, treat as a generic model response event.
	// TODO: implement compaction marker detection in ToEnvelope.
	"AfterModel":          events.EventPostCompact,
	"Notification":        events.EventNotification,
	"PreCompress":         events.EventPreCompact,
	"BeforeToolSelection": events.EventBeforeToolSelection,
}

// nervToGemini maps nerv canonical names to Gemini native event names.
var nervToGemini = map[events.Event]string{
	events.EventSessionStart:        "SessionStart",
	events.EventSessionEnd:          "SessionEnd",
	events.EventPreToolUse:          "BeforeTool",
	events.EventPostToolUse:         "AfterTool",
	events.EventSubagentStart:       "BeforeAgent",
	events.EventSubagentStop:        "AfterAgent",
	events.EventUserPromptSubmit:    "BeforeModel",
	events.EventNotification:        "Notification",
	events.EventPreCompact:          "PreCompress",
	events.EventBeforeToolSelection: "BeforeToolSelection",
}

// nervActionToGemini maps nerv merger actions to Gemini decision strings.
// Gemini uses "deny" instead of "block".
var nervActionToGemini = map[merger.Action]string{
	merger.Allow:   "allow",
	merger.Warn:    "allow", // warn has no Gemini equivalent; allow + message
	merger.Block:   "deny",
	merger.Rewrite: "allow",
}

// ToEnvelope converts a Gemini CLI event (parsed JSON map) into a nerv
// Envelope. Returns an error if the event name is unrecognized or the
// payload is malformed.
func ToEnvelope(geminiEvent map[string]any) (*envelope.Envelope, error) {
	rawName, ok := geminiEvent["hook_event_name"].(string)
	if !ok || rawName == "" {
		return nil, fmt.Errorf("gemini: missing hook_event_name")
	}

	nervEvent, known := geminiToNerv[rawName]
	if !known {
		return nil, fmt.Errorf("gemini: unknown event %q", rawName)
	}

	// Extract session info.
	var sess envelope.SessionInfo
	if sid, ok := geminiEvent["session_id"].(string); ok {
		sess.ID = sid
	}
	if cwd, ok := geminiEvent["cwd"].(string); ok {
		sess.CWD = cwd
	}

	// Extract timestamp.
	ts := time.Now()
	if raw, ok := geminiEvent["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			ts = parsed
		}
	}

	// Marshal the full Gemini event as the payload.
	payload, err := json.Marshal(geminiEvent)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal payload: %w", err)
	}

	env := &envelope.Envelope{
		NervVersion: "1.0-draft",
		Event:       string(nervEvent),
		Timestamp:   ts,
		CLI: envelope.CLIInfo{
			Name: "gemini",
		},
		Session: sess,
		Payload: payload,
	}

	return env, nil
}

// geminiDecision is the Gemini CLI stdout decision format.
type geminiDecision struct {
	Decision        string         `json:"decision"`
	Reason          string         `json:"reason,omitempty"`
	Continue        *bool          `json:"continue,omitempty"`
	StopReason      string         `json:"stopReason,omitempty"`
	SuppressOutput  *bool          `json:"suppressOutput,omitempty"`
	SystemMessage   string         `json:"systemMessage,omitempty"`
	HookSpecificOut map[string]any `json:"hookSpecificOutput,omitempty"`
}

// FromDecision converts a nerv merger Decision into a Gemini-native
// decision map and exit code. Gemini uses "deny" over "block" and
// exit code 2 for blocking.
func FromDecision(d merger.Decision) (map[string]any, int, error) {
	gemAction, ok := nervActionToGemini[d.Action]
	if !ok {
		return nil, 0, fmt.Errorf(
			"gemini: unsupported action %q", d.Action,
		)
	}

	exitCode := 0
	if d.Action == merger.Block {
		exitCode = 2
	}

	out := geminiDecision{
		Decision: gemAction,
	}

	if d.Message != "" {
		out.Reason = d.Message
		if d.Action == merger.Warn {
			out.SystemMessage = d.Message
		}
	}

	if d.Rewrite != nil {
		if out.HookSpecificOut == nil {
			out.HookSpecificOut = make(map[string]any)
		}
		out.HookSpecificOut["tool_input"] = d.Rewrite
	}

	// Marshal to map for flexible serialization.
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, 0, fmt.Errorf("gemini: marshal decision: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, 0, fmt.Errorf("gemini: unmarshal decision: %w", err)
	}

	return m, exitCode, nil
}

// TranslateEventName converts a Gemini event name to nerv canonical
// name. Returns the input unchanged if no mapping exists.
func TranslateEventName(geminiName string) string {
	if nerv, ok := geminiToNerv[geminiName]; ok {
		return string(nerv)
	}
	return geminiName
}

// ReverseEventName converts a nerv event name to a Gemini native name.
// Returns empty string if no reverse mapping exists.
func ReverseEventName(nervName events.Event) string {
	return nervToGemini[nervName]
}
