// Package claude implements the nerv adapter for Claude Code CLI.
package claude

import (
	"encoding/json"
	"fmt"
	"time"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/events"
)

// Translator converts between Claude Code hook format and nerv envelopes.
type Translator struct{}

// Decision represents a nerv hook decision (allow/block/rewrite).
type Decision struct {
	Action  string          `json:"action"` // "allow", "block", "rewrite"
	Reason  string          `json:"reason,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// ToEnvelope converts a Claude Code hook event (stdin JSON) to a nerv
// envelope. Claude events map 1:1 to nerv events — no synthesis needed.
func (t *Translator) ToEnvelope(
	claudeEvent map[string]any,
) (*envelope.Envelope, error) {
	eventName, ok := claudeEvent["hook_event_name"].(string)
	if !ok || eventName == "" {
		return nil, fmt.Errorf(
			"claude: missing or empty hook_event_name",
		)
	}

	nervEvent := events.Event(eventName)
	if !events.IsValid(nervEvent) {
		return nil, fmt.Errorf(
			"claude: unknown event %q", eventName,
		)
	}

	// Build payload from the full Claude event map. Envelope-level fields
	// (session_id, cwd, etc.) are duplicated here for standalone consumption.
	payload, err := json.Marshal(claudeEvent)
	if err != nil {
		return nil, fmt.Errorf("claude: marshal payload: %w", err)
	}

	sessionID, _ := claudeEvent["session_id"].(string)
	cwd, _ := claudeEvent["cwd"].(string)

	env := &envelope.Envelope{
		NervVersion: "1.0-draft",
		Event:       eventName,
		EventID:     fmt.Sprintf("claude-%s-%d", eventName, time.Now().UnixNano()),
		Timestamp:   time.Now().UTC(),
		CLI:         envelope.CLIInfo{Name: "claude"},
		Session:     envelope.SessionInfo{ID: sessionID, CWD: cwd},
		Payload:     payload,
		Synthesized: false,
	}
	return env, nil
}

// silentStdoutEvents are events whose stdout Claude interprets outside
// the decision schema — WorktreeCreate reads stdout as the path of a
// hook-created worktree directory. nerv only observes these events, so
// it must write nothing regardless of the merged decision.
var silentStdoutEvents = map[string]bool{
	"WorktreeCreate": true,
	"WorktreeRemove": true,
}

// FromDecision converts a nerv Decision to Claude's expected stdout JSON
// and exit code. Returns (jsonBytes, exitCode, error). A nil byte slice
// means write nothing to stdout.
//
// Exit code semantics:
//   - 0 = allow (success)
//   - 2 = block
//   - 3 = rewrite (modified payload)
//
// Warn has no Claude channel: it degrades to allow here; callers surface
// the message on stderr.
func (t *Translator) FromDecision(
	d Decision, eventName string,
) ([]byte, int, error) {
	if silentStdoutEvents[eventName] {
		return nil, 0, nil
	}

	switch d.Action {
	case "allow", "warn":
		return []byte("{}"), 0, nil

	case "block":
		out := buildBlockOutput(eventName, d.Reason)
		b, err := json.Marshal(out)
		if err != nil {
			return nil, 0, fmt.Errorf(
				"claude: marshal block decision: %w", err,
			)
		}
		return b, 2, nil

	case "rewrite":
		out, err := buildRewriteOutput(eventName, d.Payload)
		if err != nil {
			return nil, 0, err
		}
		b, err := json.Marshal(out)
		if err != nil {
			return nil, 0, fmt.Errorf(
				"claude: marshal rewrite decision: %w", err,
			)
		}
		return b, 3, nil

	default:
		return nil, 0, fmt.Errorf(
			"claude: unknown action %q", d.Action,
		)
	}
}

// buildBlockOutput creates Claude's block response per event type.
func buildBlockOutput(eventName, reason string) map[string]any {
	switch eventName {
	case "PreToolUse":
		return map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":            "PreToolUse",
				"permissionDecision":       "deny",
				"permissionDecisionReason": reason,
			},
		}
	case "PermissionRequest":
		return map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName": "PermissionRequest",
				"decision": map[string]any{
					"behavior": "deny",
					"message":  reason,
				},
			},
		}
	default:
		return map[string]any{
			"decision": "block",
			"reason":   reason,
		}
	}
}

// buildRewriteOutput creates Claude's rewrite response for PreToolUse.
func buildRewriteOutput(
	eventName string, payload json.RawMessage,
) (map[string]any, error) {
	if eventName != "PreToolUse" && eventName != "PermissionRequest" {
		return nil, fmt.Errorf(
			"claude: rewrite not supported for event %q", eventName,
		)
	}

	var input map[string]any
	if err := json.Unmarshal(payload, &input); err != nil {
		return nil, fmt.Errorf(
			"claude: unmarshal rewrite payload: %w", err,
		)
	}

	switch eventName {
	case "PreToolUse":
		return map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName":      "PreToolUse",
				"permissionDecision": "allow",
				"updatedInput":       input,
			},
		}, nil
	case "PermissionRequest":
		return map[string]any{
			"hookSpecificOutput": map[string]any{
				"hookEventName": "PermissionRequest",
				"decision": map[string]any{
					"behavior":     "allow",
					"updatedInput": input,
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf(
			"claude: rewrite not supported for event %q", eventName,
		)
	}
}
