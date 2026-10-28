// Package goose implements the nerv adapter for Goose CLI.
package goose

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// gooseToNerv maps Goose SSE/extension event names to nerv canonical names.
//
// Native SSE events from crates/goose-server/src/routes/reply.rs:
//   - Message, Error, Finish, Notification, UpdateConversation,
//     ActiveRequests, Ping
//
// Permission events from crates/goose/src/permission/:
//   - action_required, permission.deny_once, permission.always_deny
//
// Subagent events from notification_events.rs:
//   - subagent_created, subagent_completed, subagent_terminated
//
// Task events from TaskExecutionNotificationEvent:
//   - tasks_update, tasks_complete
var gooseToNerv = map[string]events.Event{
	// --- native SSE events ---
	"session.start":       events.EventSessionStart,
	"session.end":         events.EventSessionEnd,
	"finish":              events.EventStop,
	"notification":        events.EventNotification,
	"update_conversation": events.EventPostCompact,
	"action_required":     events.EventPermissionRequest,
	"permission_denied":   events.EventPermissionDenied,

	// --- subagent events (summon) ---
	"subagent_created":    events.EventSubagentStart,
	"subagent_completed":  events.EventSubagentStop,
	"subagent_terminated": events.EventSubagentStop,

	// --- task events ---
	"tasks_complete": events.EventTaskCompleted,
	"tasks_update":   events.EventTaskCreated,

	// --- extension/bridge events ---
	"tool.before":         events.EventPreToolUse,
	"tool.after":          events.EventPostToolUse,
	"tool.error":          events.EventPostToolUseFailure,
	"file.changed":        events.EventFileChanged,
	"cwd.changed":         events.EventCwdChanged,
	"worktree.created":    events.EventWorktreeCreate,
	"worktree.removed":    events.EventWorktreeRemove,
	"compaction.request":  events.EventPreCompact,
	"instructions.loaded": events.EventInstructionsLoaded,

	// --- approximate mappings ---
	"request.start": events.EventUserPromptSubmit,
	"error":         events.EventStopFailure,
}

// nervToGoose maps nerv canonical names to Goose event names.
var nervToGoose = map[events.Event]string{
	events.EventSessionStart:       "session.start",
	events.EventSessionEnd:         "session.end",
	events.EventStop:               "finish",
	events.EventNotification:       "notification",
	events.EventPostCompact:        "update_conversation",
	events.EventPermissionRequest:  "action_required",
	events.EventPermissionDenied:   "permission_denied",
	events.EventSubagentStart:      "subagent_created",
	events.EventSubagentStop:       "subagent_completed",
	events.EventTaskCompleted:      "tasks_complete",
	events.EventTaskCreated:        "tasks_update",
	events.EventPreToolUse:         "tool.before",
	events.EventPostToolUse:        "tool.after",
	events.EventPostToolUseFailure: "tool.error",
	events.EventFileChanged:        "file.changed",
	events.EventCwdChanged:         "cwd.changed",
	events.EventWorktreeCreate:     "worktree.created",
	events.EventWorktreeRemove:     "worktree.removed",
	events.EventPreCompact:         "compaction.request",
	events.EventInstructionsLoaded: "instructions.loaded",
	events.EventUserPromptSubmit:   "request.start",
	events.EventStopFailure:        "error",
}

// nativeEvents lists Goose events that are emitted natively (SSE or
// permission system) rather than synthesized by the nerv bridge extension.
var nativeEvents = map[string]bool{
	"session.start":       true,
	"session.end":         true,
	"finish":              true,
	"notification":        true,
	"update_conversation": true,
	"action_required":     true,
	"permission_denied":   true,
	"subagent_created":    true,
	"subagent_completed":  true,
	"subagent_terminated": true,
	"tasks_complete":      true,
	"error":               true,
}

// nervActionToGoose maps nerv merger actions to Goose decision strings.
// Goose uses "continue" for allow/warn and "cancel" for block.
// Exit codes: 0=allow, 2=block (matching Goose Permission semantics).
var nervActionToGoose = map[merger.Action]string{
	merger.Allow:   "continue",
	merger.Warn:    "continue",
	merger.Block:   "cancel",
	merger.Rewrite: "continue",
}

// ToEnvelope converts a Goose CLI event (parsed JSON map) into a nerv
// Envelope. Returns an error if the event name is unrecognized.
func ToEnvelope(gooseEvent map[string]any) (*envelope.Envelope, error) {
	rawName, ok := gooseEvent["type"].(string)
	if !ok || rawName == "" {
		// Fall back to hook_event_name for bridge compatibility.
		rawName, ok = gooseEvent["hook_event_name"].(string)
		if !ok || rawName == "" {
			return nil, fmt.Errorf("goose: missing type or hook_event_name")
		}
	}

	nervEvent, known := gooseToNerv[rawName]
	if !known {
		return nil, fmt.Errorf("goose: unknown event %q", rawName)
	}

	// tasks_update only fires TaskCreated on Pending->Running
	// transition; other statuses map to Notification.
	if rawName == "tasks_update" {
		status, _ := gooseEvent["status"].(string)
		if status != "Running" {
			nervEvent = events.EventNotification
		}
	}

	// Refine tool.after into more specific nerv events based on tool
	// name and command content (shell cd, worktree ops, file writes).
	if synth := synthesizeFromToolAfter(rawName, gooseEvent); synth != "" {
		nervEvent = synth
	}

	var sess envelope.SessionInfo
	if sid, ok := gooseEvent["session_id"].(string); ok {
		sess.ID = sid
	}
	if cwd, ok := gooseEvent["cwd"].(string); ok {
		sess.CWD = cwd
	}

	ts := time.Now()
	if raw, ok := gooseEvent["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			ts = parsed
		}
	}

	payload, err := json.Marshal(gooseEvent)
	if err != nil {
		return nil, fmt.Errorf("goose: marshal payload: %w", err)
	}

	synthesized := !nativeEvents[rawName]

	env := &envelope.Envelope{
		NervVersion: "1.0-draft",
		Event:       string(nervEvent),
		Timestamp:   ts,
		Synthesized: synthesized,
		CLI: envelope.CLIInfo{
			Name: "goose",
		},
		Session: sess,
		Payload: payload,
	}

	if synthesized {
		src := synthesisSourceFor(rawName)
		env.SynthesisSource = &src
	}

	return env, nil
}

// synthesisSourceFor returns the correct synthesis_source for a given
// Goose event name. Events from the nerv bridge extension get
// "extension"; SSE-derived approximations get "sse"; shell-heuristic
// events get "shell-match"; notification-derived events get
// "notification"; signal-derived events get "signal".
func synthesisSourceFor(rawName string) string {
	switch rawName {
	case "request.start":
		return "sse"
	case "tasks_update":
		return "notification"
	case "cwd.changed", "worktree.created", "worktree.removed":
		return "shell-match"
	case "compaction.request":
		return "signal"
	default:
		return "extension"
	}
}

// synthesizeFromToolAfter detects additional nerv events from Goose
// developer extension shell/file operations embedded in tool.after.
func synthesizeFromToolAfter(
	rawName string, ev map[string]any,
) events.Event {
	if rawName != "tool.after" {
		return ""
	}

	toolName, _ := ev["tool_name"].(string)
	cmd := extractCommand(ev)

	switch toolName {
	case "developer__shell":
		if strings.HasPrefix(cmd, "cd ") {
			return events.EventCwdChanged
		}
		if strings.Contains(cmd, "worktree add") ||
			strings.Contains(cmd, "git hop add") {
			return events.EventWorktreeCreate
		}
		if strings.Contains(cmd, "worktree remove") ||
			strings.Contains(cmd, "git hop remove") {
			return events.EventWorktreeRemove
		}
	case "developer__text_editor", "developer__write_file":
		return events.EventFileChanged
	}

	return ""
}

// extractCommand pulls the command string from tool_input.
func extractCommand(ev map[string]any) string {
	if ti, ok := ev["tool_input"].(map[string]any); ok {
		if c, ok := ti["command"].(string); ok {
			return c
		}
	}
	return ""
}

// gooseDecision is the Goose extension stdout decision format.
type gooseDecision struct {
	Action  string `json:"action"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// FromDecision converts a nerv merger Decision into a Goose-native
// decision map and exit code.
//
// Exit code semantics (aligned with Goose Permission enum):
//   - 0 = allow (AllowOnce / AlwaysAllow)
//   - 2 = block (DenyOnce / AlwaysDeny / Cancel)
func FromDecision(d merger.Decision) (map[string]any, int, error) {
	gooseAction, ok := nervActionToGoose[d.Action]
	if !ok {
		return nil, 0, fmt.Errorf(
			"goose: unsupported action %q", d.Action,
		)
	}

	exitCode := 0
	if d.Action == merger.Block {
		exitCode = 2
	}

	out := gooseDecision{
		Action: gooseAction,
	}
	if d.Message != "" {
		out.Reason = d.Message
		if d.Action == merger.Warn {
			out.Message = d.Message
		}
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return nil, 0, fmt.Errorf("goose: marshal decision: %w", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, 0, fmt.Errorf("goose: unmarshal decision: %w", err)
	}

	return m, exitCode, nil
}

// TranslateEventName converts a Goose event name to nerv canonical
// name. Returns the input unchanged if no mapping exists.
func TranslateEventName(gooseName string) string {
	if nerv, ok := gooseToNerv[gooseName]; ok {
		return string(nerv)
	}
	return gooseName
}

// ReverseEventName converts a nerv event name to a Goose event name.
// Returns empty string if no reverse mapping exists.
func ReverseEventName(nervName events.Event) string {
	return nervToGoose[nervName]
}
