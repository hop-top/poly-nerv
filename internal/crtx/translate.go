package crtx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/events"
)

// Emission is one bus publish produced from a hook envelope.
type Emission struct {
	Topic   string
	Payload any
}

// kindNormalization maps hook CLI names onto the source kinds the crtx
// spec recommends (§5). Unlisted names pass through verbatim — kind is
// an open string.
var kindNormalization = map[string]string{
	"claude": "claude-code",
	"gemini": "gemini-cli",
}

// Translate maps a hook envelope onto zero or more crtx bus emissions
// per the mapping table in docs/architecture/crtx-producer.md. Events
// without a conversation-shaped counterpart translate to nothing;
// they keep flowing through dispatch and the internal eventlog.
func Translate(env *envelope.Envelope) []Emission {
	if env == nil {
		return nil
	}

	switch events.Event(env.Event) {
	case events.EventSessionStart:
		return one(TopicSessionCreated, SessionCreatedPayload{
			EnvelopeID: env.Session.ID,
			CreatedAt:  stamp(env),
			Source:     sourceOf(env),
		})

	case events.EventSessionEnd:
		return one(TopicVendorSessionEnded, vendorPayload(env))

	case events.EventUserPromptSubmit:
		var p struct {
			Prompt string `json:"prompt"`
		}
		decode(env.Payload, &p)
		return one(TopicUserReceived, UserReceivedPayload{
			EnvelopeID: env.Session.ID,
			TurnID:     env.EventID,
			Content:    []any{NewTextPart(p.Prompt)},
		})

	case events.EventPreToolUse:
		var p toolPayload
		decode(env.Payload, &p)
		return one(TopicToolCalled, ToolCalledPayload{
			EnvelopeID: env.Session.ID,
			TurnID:     env.EventID,
			CallID:     toolCallID(env.Session.ID, p.ToolName, p.ToolInput),
			Name:       p.ToolName,
			Input:      coerceInput(p.ToolInput),
		})

	case events.EventPostToolUse:
		var p toolPayload
		decode(env.Payload, &p)
		return one(TopicToolCompleted, ToolCompletedPayload{
			EnvelopeID: env.Session.ID,
			TurnID:     env.EventID,
			CallID:     toolCallID(env.Session.ID, p.ToolName, p.ToolInput),
			Output:     rawOutput(p.ToolResponse),
		})

	case events.EventPostToolUseFailure:
		var p toolPayload
		decode(env.Payload, &p)
		return one(TopicToolCompleted, ToolCompletedPayload{
			EnvelopeID: env.Session.ID,
			TurnID:     env.EventID,
			CallID:     toolCallID(env.Session.ID, p.ToolName, p.ToolInput),
			Output:     p.Error,
			IsError:    true,
		})

	case events.EventStop:
		var p struct {
			StopReason string `json:"stop_reason"`
			TurnIndex  *int   `json:"turn_index"`
		}
		decode(env.Payload, &p)
		part := ExtensionPart{"type": StopPartType, "stop_reason": p.StopReason}
		if p.TurnIndex != nil {
			part["turn_index"] = *p.TurnIndex
		}
		return one(TopicAssistantEmitted, AssistantEmittedPayload{
			EnvelopeID: env.Session.ID,
			TurnID:     env.EventID,
			Content:    []any{part},
		})

	case events.EventStopFailure:
		return one(TopicVendorTurnFailed, vendorPayload(env))

	case events.EventSubagentStart:
		var p subagentPayload
		decode(env.Payload, &p)
		parent := p.parentOr(env.Session.ID)
		return one(TopicSessionNested, SessionNestedPayload{
			ParentID:  parent,
			ChildID:   p.AgentID,
			CallID:    nestCallID(parent, p.AgentID),
			CreatedAt: stamp(env),
		})

	case events.EventSubagentStop:
		var p subagentPayload
		decode(env.Payload, &p)
		parent := p.parentOr(env.Session.ID)
		return one(TopicSessionReturned, SessionReturnedPayload{
			ParentID:  parent,
			ChildID:   p.AgentID,
			CallID:    nestCallID(parent, p.AgentID),
			CreatedAt: stamp(env),
			IsError:   p.ExitCode != nil && *p.ExitCode != 0,
		})

	case events.EventNotification:
		return one(TopicVendorNotificationEmitted, vendorPayload(env))
	case events.EventPermissionRequest:
		return one(TopicVendorPermissionRequested, vendorPayload(env))
	case events.EventPermissionDenied:
		return one(TopicVendorPermissionDenied, vendorPayload(env))
	case events.EventTaskCreated:
		return one(TopicVendorTaskCreated, vendorPayload(env))
	case events.EventTaskCompleted:
		return one(TopicVendorTaskCompleted, vendorPayload(env))
	}

	// Everything else is workspace / host telemetry: dropped from the
	// crtx surface by decision (see the ADR's mapping table).
	return nil
}

// toolPayload covers PreToolUse, PostToolUse, and PostToolUseFailure.
type toolPayload struct {
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
	Error        string          `json:"error"`
}

// subagentPayload covers SubagentStart and SubagentStop.
type subagentPayload struct {
	AgentID         string `json:"agent_id"`
	ParentSessionID string `json:"parent_session_id"`
	ExitCode        *int   `json:"exit_code"`
}

func (p subagentPayload) parentOr(fallback string) string {
	if p.ParentSessionID != "" {
		return p.ParentSessionID
	}
	return fallback
}

func one(topic string, payload any) []Emission {
	return []Emission{{Topic: topic, Payload: payload}}
}

// decode is a tolerant unmarshal: a missing or malformed hook payload
// leaves the target zero-valued instead of failing the translation
// (fail-open — the emission still carries envelope identity).
func decode(raw json.RawMessage, into any) {
	if len(raw) == 0 {
		return
	}
	_ = json.Unmarshal(raw, into)
}

func stamp(env *envelope.Envelope) string {
	return env.Timestamp.UTC().Format(time.RFC3339Nano)
}

func sourceOf(env *envelope.Envelope) Source {
	kind := env.CLI.Name
	if n, ok := kindNormalization[kind]; ok {
		kind = n
	}
	if kind == "" {
		kind = "custom"
	}
	version := env.CLI.Version
	if version == "" {
		version = "unknown"
	}
	return Source{Kind: kind, Version: version}
}

func vendorPayload(env *envelope.Envelope) VendorPayload {
	return VendorPayload{
		EnvelopeID: env.Session.ID,
		Event:      env.Event,
		EventID:    env.EventID,
		OccurredAt: stamp(env),
		Payload:    env.Payload,
	}
}

// sep separates digest fields; 0x1f (unit separator) cannot appear in
// tool names or session ids and keeps concatenations unambiguous.
const sep = "\x1f"

// toolCallID derives the deterministic call id pairing tool.called
// with tool.completed: the hook wire format carries no correlation
// key, so both sides hash (session, tool, canonical input). See the
// ADR for the collision trade-off on byte-identical repeated calls.
func toolCallID(sessionID, toolName string, input json.RawMessage) string {
	return "c" + digest(sessionID+sep+toolName+sep+canonicalJSON(input))
}

// nestCallID derives the synthetic call id pairing envelope.nested
// with envelope.returned for a subagent.
func nestCallID(parentID, childID string) string {
	return "n" + digest(parentID+sep+childID)
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:12])
}

// canonicalJSON re-encodes raw with sorted object keys (encoding/json
// sorts map keys) so digests are stable across producers that order
// fields differently. Malformed input falls back to the raw bytes.
func canonicalJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(out)
}

// coerceInput shapes tool input for the schema's object|string
// constraint: objects and strings pass through decoded; anything else
// (missing, array, number) is stringified.
func coerceInput(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	switch v.(type) {
	case map[string]any, string:
		return v
	case nil:
		return map[string]any{}
	default:
		return string(raw)
	}
}

// rawOutput decodes a tool response for the `output` field (any JSON
// value is legal). Missing responses become null-equivalent empty
// objects so the required key stays present and meaningful.
func rawOutput(raw json.RawMessage) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	return v
}
