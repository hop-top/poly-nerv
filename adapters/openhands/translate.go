// Package openhands implements the nerv adapter for OpenHands
// (formerly OpenDevin). OpenHands uses a Claude Code-compatible
// hooks system (PR #1467 in software-agent-sdk).
package openhands

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// openhandsToNerv maps OpenHands native hook event names to nerv
// canonical names. Native hooks use Claude Code-compatible names.
var openhandsToNerv = map[string]events.Event{
	// Native hook events (wired in PR #1467)
	"PreToolUse":       events.EventPreToolUse,
	"PostToolUse":      events.EventPostToolUse,
	"UserPromptSubmit": events.EventUserPromptSubmit,
	"SessionStart":     events.EventSessionStart,
	"SessionEnd":       events.EventSessionEnd,
	"Stop":             events.EventStop,
}

// nervToOpenhands maps nerv canonical names to OpenHands event names.
var nervToOpenhands = map[events.Event]string{
	events.EventPreToolUse:       "PreToolUse",
	events.EventPostToolUse:      "PostToolUse",
	events.EventUserPromptSubmit: "UserPromptSubmit",
	events.EventSessionStart:     "SessionStart",
	events.EventSessionEnd:       "SessionEnd",
	events.EventStop:             "Stop",
}

// nervActionToOpenhands maps nerv merger actions to OpenHands decisions.
// Exit code semantics match Claude Code: 0=allow, 2=block.
var nervActionToOpenhands = map[merger.Action]string{
	merger.Allow:   "allow",
	merger.Warn:    "allow",
	merger.Block:   "block",
	merger.Rewrite: "allow",
}

// ToEnvelope converts an OpenHands event (parsed JSON map) into a nerv
// Envelope. OpenHands native hooks use "hook_event_name" as the event
// field (Claude Code-compatible).
func ToEnvelope(ohEvent map[string]any) (*envelope.Envelope, error) {
	// Try Claude Code-compatible field first, fall back to "event".
	rawName, ok := ohEvent["hook_event_name"].(string)
	if !ok || rawName == "" {
		rawName, ok = ohEvent["event"].(string)
		if !ok || rawName == "" {
			return nil, fmt.Errorf(
				"openhands: missing hook_event_name/event field",
			)
		}
	}

	nervEvent, known := openhandsToNerv[rawName]
	if !known {
		return nil, fmt.Errorf(
			"openhands: unknown event %q", rawName,
		)
	}

	// Extract session info.
	var sess envelope.SessionInfo
	if sid, ok := ohEvent["session_id"].(string); ok {
		sess.ID = sid
	}
	if cwd, ok := ohEvent["cwd"].(string); ok {
		sess.CWD = cwd
	}

	// Extract timestamp.
	ts := time.Now()
	if raw, ok := ohEvent["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			ts = parsed
		}
	}

	// Marshal the full event as the payload.
	payload, err := json.Marshal(ohEvent)
	if err != nil {
		return nil, fmt.Errorf("openhands: marshal payload: %w", err)
	}

	env := &envelope.Envelope{
		NervVersion: "1.0-draft",
		Event:       string(nervEvent),
		Timestamp:   ts,
		CLI: envelope.CLIInfo{
			Name: "openhands",
		},
		Session: sess,
		Payload: payload,
	}

	return env, nil
}

// openhandsDecision is the OpenHands stdout decision format.
// Compatible with Claude Code's action/reason schema.
type openhandsDecision struct {
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

// FromDecision converts a nerv merger Decision into an OpenHands-native
// decision map and exit code. Exit codes match Claude Code: 0=allow,
// 2=block.
func FromDecision(d merger.Decision) (map[string]any, int, error) {
	ohAction, ok := nervActionToOpenhands[d.Action]
	if !ok {
		return nil, 0, fmt.Errorf(
			"openhands: unsupported action %q", d.Action,
		)
	}

	exitCode := 0
	if d.Action == merger.Block {
		exitCode = 2
	}

	out := openhandsDecision{
		Action: ohAction,
	}

	if d.Message != "" {
		out.Reason = d.Message
	}

	raw, err := json.Marshal(out)
	if err != nil {
		return nil, 0, fmt.Errorf(
			"openhands: marshal decision: %w", err,
		)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, 0, fmt.Errorf(
			"openhands: unmarshal decision: %w", err,
		)
	}

	return m, exitCode, nil
}

// TranslateEventName converts an OpenHands event name to nerv
// canonical name. Returns the input unchanged if no mapping exists.
func TranslateEventName(ohName string) string {
	if nerv, ok := openhandsToNerv[ohName]; ok {
		return string(nerv)
	}
	return ohName
}

// ReverseEventName converts a nerv event name to an OpenHands native
// name. Returns empty string if no reverse mapping exists.
func ReverseEventName(nervName events.Event) string {
	return nervToOpenhands[nervName]
}

// SyntheticEvent represents a nerv event synthesized from OpenHands
// internal event stream (Actions/Observations).
type SyntheticEvent struct {
	Event   events.Event
	Payload map[string]any
}

// Regex patterns for synthesis matchers.
var (
	cdPattern    = regexp.MustCompile(`(?m)^\s*cd\s+`)
	writeToolPat = regexp.MustCompile(
		`(?i)^(FileWrite|FileEdit|write|edit|apply_patch)$`,
	)
)

// SynthesizeFromEventStream inspects an OpenHands internal event
// (Action/Observation) and returns zero or more synthetic nerv events.
// These are derived from the 21 Action + 19 Observation types.
func SynthesizeFromEventStream(payload map[string]any) []SyntheticEvent {
	action := strVal(payload, "action")
	observation := strVal(payload, "observation")

	var out []SyntheticEvent

	// AgentDelegateAction → SubagentStart
	if action == "AgentDelegateAction" {
		out = append(out, SyntheticEvent{
			Event: events.EventSubagentStart,
			Payload: map[string]any{
				"agent": strVal(payload, "agent"),
			},
		})
	}

	// AgentDelegateObservation → SubagentStop
	if observation == "AgentDelegateObservation" {
		out = append(out, SyntheticEvent{
			Event: events.EventSubagentStop,
			Payload: map[string]any{
				"agent": strVal(payload, "agent"),
			},
		})
	}

	// FileWrite/EditObservation → FileChanged
	if writeToolPat.MatchString(observation) {
		path := strVal(payload, "path")
		if path != "" {
			out = append(out, SyntheticEvent{
				Event: events.EventFileChanged,
				Payload: map[string]any{
					"path":        path,
					"observation": observation,
				},
			})
		}
	}

	// CmdRunAction with cd → CwdChanged
	if action == "CmdRunAction" {
		cmd := strVal(payload, "command")
		if cmd != "" && cdPattern.MatchString(cmd) {
			out = append(out, SyntheticEvent{
				Event:   events.EventCwdChanged,
				Payload: map[string]any{"command": cmd},
			})
		}
	}

	// CondensationAction → PreCompact/PostCompact
	if action == "CondensationAction" {
		out = append(out, SyntheticEvent{
			Event:   events.EventPreCompact,
			Payload: map[string]any{},
		})
	}
	if observation == "CondensationObservation" {
		out = append(out, SyntheticEvent{
			Event:   events.EventPostCompact,
			Payload: map[string]any{},
		})
	}

	// ErrorObservation → StopFailure or PostToolUseFailure.
	// Agent/session errors (no tool context, or FINISH/CHANGE_AGENT_STATE
	// action) map to StopFailure; tool-related errors map to
	// PostToolUseFailure.
	if observation == "ErrorObservation" {
		actionType := strVal(payload, "action_type")
		source := strVal(payload, "source")
		toolName := strVal(payload, "tool_name")

		isAgentError := source == "agent" ||
			actionType == "FINISH" ||
			actionType == "CHANGE_AGENT_STATE"
		isToolError := toolName != ""

		if isAgentError && !isToolError {
			out = append(out, SyntheticEvent{
				Event: events.EventStopFailure,
				Payload: map[string]any{
					"error": strVal(payload, "error"),
				},
			})
		} else {
			out = append(out, SyntheticEvent{
				Event: events.EventPostToolUseFailure,
				Payload: map[string]any{
					"error": strVal(payload, "error"),
				},
			})
		}
	}

	// RecallObservation → InstructionsLoaded
	if observation == "RecallObservation" {
		out = append(out, SyntheticEvent{
			Event:   events.EventInstructionsLoaded,
			Payload: map[string]any{},
		})
	}

	// UserRejectObservation → PermissionDenied
	if observation == "UserRejectObservation" {
		out = append(out, SyntheticEvent{
			Event:   events.EventPermissionDenied,
			Payload: map[string]any{},
		})
	}

	// TaskTrackingAction → TaskCreated
	if action == "TaskTrackingAction" {
		out = append(out, SyntheticEvent{
			Event: events.EventTaskCreated,
			Payload: map[string]any{
				"task": strVal(payload, "task"),
			},
		})
	}

	// TaskTrackingObservation → TaskCompleted
	if observation == "TaskTrackingObservation" {
		out = append(out, SyntheticEvent{
			Event: events.EventTaskCompleted,
			Payload: map[string]any{
				"task": strVal(payload, "task"),
			},
		})
	}

	// MCPAction → Elicitation
	if action == "MCPAction" {
		out = append(out, SyntheticEvent{
			Event:   events.EventElicitation,
			Payload: map[string]any{},
		})
	}

	// MCPObservation → ElicitationResult
	if observation == "MCPObservation" {
		out = append(out, SyntheticEvent{
			Event:   events.EventElicitationResult,
			Payload: map[string]any{},
		})
	}

	return out
}

// --- helpers ---

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}
