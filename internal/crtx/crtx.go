// Package crtx declares the crtx v0.1 producer surface for nerv: the
// canonical bus topics, the event payload and content-part types, and
// the translation from hook envelopes onto them.
//
// The type surface is declared locally on purpose — the JSON Schema
// published by the crtx spec is the contract of record, not any Go
// module; the vendored copies under testdata/ pin these types to it.
// See docs/architecture/crtx-producer.md for the full rationale and
// the event mapping table.
package crtx

import "encoding/json"

// Version is the crtx spec version this producer emits. Pinned; the
// vendored v0.1 schemas are the only ones the tests validate against.
const Version = "0.1"

// Canonical bus topics from the crtx v0.1 events spec §3. The crtx.*
// namespace is reserved by the spec — never invent additional topics
// under it.
const (
	TopicUserReceived     = "crtx.turn.user.received"
	TopicAssistantEmitted = "crtx.turn.assistant.emitted"
	TopicToolCalled       = "crtx.turn.tool.called"
	TopicToolCompleted    = "crtx.turn.tool.completed"
	TopicThinkingEmitted  = "crtx.turn.thinking.emitted"
	TopicSystemInjected   = "crtx.turn.system.injected"
	TopicSessionCreated   = "crtx.session.envelope.created"
	TopicSessionForked    = "crtx.session.envelope.forked"
	TopicSessionNested    = "crtx.session.envelope.nested"
	TopicSessionReturned  = "crtx.session.envelope.returned"
)

// Vendor extension topics (events spec §5). Four segments so one name
// satisfies both the crtx vendor-prefix rule and the kit/bus
// [Source].[Category].[Object].[Action] publish grammar.
const (
	TopicVendorSessionEnded        = "x.nerv.session.ended"
	TopicVendorTurnFailed          = "x.nerv.turn.failed"
	TopicVendorNotificationEmitted = "x.nerv.notification.emitted"
	TopicVendorPermissionRequested = "x.nerv.permission.requested"
	TopicVendorPermissionDenied    = "x.nerv.permission.denied"
	TopicVendorTaskCreated         = "x.nerv.task.created"
	TopicVendorTaskCompleted       = "x.nerv.task.completed"

	// Inbox lifecycle topics for the guardrails escalation path. The
	// nerv event names in spec/events.yaml keep their snake_case form;
	// only the bus topic carries the vendor-prefixed dotted shape.
	TopicVendorInboxMessageAdded    = "x.nerv.inbox_message.added"
	TopicVendorInboxMessageUpdated  = "x.nerv.inbox_message.updated"
	TopicVendorInboxMessagePromoted = "x.nerv.inbox_message.promoted"
	TopicVendorInboxMessageDropped  = "x.nerv.inbox_message.dropped"
)

// StopPartType is the extension ContentPart type carried by
// assistant.emitted events: the hook layer observes turn completion
// (stop reason, turn index) but never the assistant's text.
const StopPartType = "x-io.hop-top.nerv.stop"

// BusSource is the kit/bus Event.Source value for every publish from
// this producer.
const BusSource = "nerv"

// AllTopics returns every topic this producer can emit, canonical and
// vendor. The slice is a fresh copy.
func AllTopics() []string {
	return []string{
		TopicUserReceived,
		TopicAssistantEmitted,
		TopicToolCalled,
		TopicToolCompleted,
		TopicThinkingEmitted,
		TopicSystemInjected,
		TopicSessionCreated,
		TopicSessionForked,
		TopicSessionNested,
		TopicSessionReturned,
		TopicVendorSessionEnded,
		TopicVendorTurnFailed,
		TopicVendorNotificationEmitted,
		TopicVendorPermissionRequested,
		TopicVendorPermissionDenied,
		TopicVendorTaskCreated,
		TopicVendorTaskCompleted,
		TopicVendorInboxMessageAdded,
		TopicVendorInboxMessageUpdated,
		TopicVendorInboxMessagePromoted,
		TopicVendorInboxMessageDropped,
	}
}

// Source identifies the runtime that produced an Envelope (spec §5).
type Source struct {
	Kind     string `json:"kind"`
	Version  string `json:"version"`
	Instance string `json:"instance,omitempty"`
}

// TextPart is the `text` ContentPart (spec §6.1).
type TextPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// NewTextPart returns a TextPart wrapping s.
func NewTextPart(s string) TextPart {
	return TextPart{Type: "text", Text: s}
}

// ToolCallPart is the `tool_call` ContentPart (spec §6.2).
type ToolCallPart struct {
	Type         string `json:"type"`
	CallID       string `json:"call_id"`
	Name         string `json:"name"`
	Input        any    `json:"input"`
	ParentCallID string `json:"parent_call_id,omitempty"`
}

// ToolResultPart is the `tool_result` ContentPart (spec §6.3).
type ToolResultPart struct {
	Type            string `json:"type"`
	CallID          string `json:"call_id"`
	Output          any    `json:"output"`
	IsError         bool   `json:"is_error,omitempty"`
	ChildEnvelopeID string `json:"child_envelope_id,omitempty"`
}

// ExtensionPart is an `x-`-prefixed ContentPart (spec §6.6). The map
// MUST contain a "type" key matching ^x-[a-zA-Z0-9._-]+$.
type ExtensionPart map[string]any

// UserReceivedPayload is the crtx.turn.user.received payload.
type UserReceivedPayload struct {
	EnvelopeID      string `json:"envelope_id"`
	TurnID          string `json:"turn_id"`
	Content         []any  `json:"content"`
	InReplyToCallID string `json:"in_reply_to_call_id,omitempty"`
}

// AssistantEmittedPayload is the crtx.turn.assistant.emitted payload.
type AssistantEmittedPayload struct {
	EnvelopeID      string `json:"envelope_id"`
	TurnID          string `json:"turn_id"`
	Content         []any  `json:"content"`
	AgentID         string `json:"agent_id,omitempty"`
	InReplyToCallID string `json:"in_reply_to_call_id,omitempty"`
}

// ToolCalledPayload is the crtx.turn.tool.called payload.
type ToolCalledPayload struct {
	EnvelopeID   string `json:"envelope_id"`
	TurnID       string `json:"turn_id"`
	CallID       string `json:"call_id"`
	Name         string `json:"name"`
	Input        any    `json:"input"`
	ParentCallID string `json:"parent_call_id,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
}

// ToolCompletedPayload is the crtx.turn.tool.completed payload.
type ToolCompletedPayload struct {
	EnvelopeID      string `json:"envelope_id"`
	TurnID          string `json:"turn_id"`
	CallID          string `json:"call_id"`
	Output          any    `json:"output"`
	IsError         bool   `json:"is_error,omitempty"`
	ChildEnvelopeID string `json:"child_envelope_id,omitempty"`
	AgentID         string `json:"agent_id,omitempty"`
}

// ThinkingEmittedPayload is the crtx.turn.thinking.emitted payload.
type ThinkingEmittedPayload struct {
	EnvelopeID      string `json:"envelope_id"`
	TurnID          string `json:"turn_id"`
	Text            string `json:"text"`
	Signature       string `json:"signature,omitempty"`
	AgentID         string `json:"agent_id,omitempty"`
	InReplyToCallID string `json:"in_reply_to_call_id,omitempty"`
}

// SystemInjectedPayload is the crtx.turn.system.injected payload.
type SystemInjectedPayload struct {
	EnvelopeID      string `json:"envelope_id"`
	TurnID          string `json:"turn_id"`
	Content         []any  `json:"content"`
	InReplyToCallID string `json:"in_reply_to_call_id,omitempty"`
}

// SessionCreatedPayload is the crtx.session.envelope.created payload.
type SessionCreatedPayload struct {
	EnvelopeID string `json:"envelope_id"`
	CreatedAt  string `json:"created_at"`
	Source     Source `json:"source"`
}

// SessionNestedPayload is the crtx.session.envelope.nested payload.
type SessionNestedPayload struct {
	ParentID  string `json:"parent_id"`
	ChildID   string `json:"child_id"`
	CallID    string `json:"call_id"`
	CreatedAt string `json:"created_at"`
}

// SessionReturnedPayload is the crtx.session.envelope.returned payload.
type SessionReturnedPayload struct {
	ParentID  string `json:"parent_id"`
	ChildID   string `json:"child_id"`
	CallID    string `json:"call_id"`
	CreatedAt string `json:"created_at"`
	IsError   bool   `json:"is_error,omitempty"`
}

// VendorPayload is the uniform payload shape for every x.nerv.* topic
// (extension payloads are caller-defined per events spec §5). Payload
// preserves the nerv event payload verbatim.
type VendorPayload struct {
	EnvelopeID string          `json:"envelope_id"`
	Event      string          `json:"event"`
	EventID    string          `json:"event_id"`
	OccurredAt string          `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}
