package ingest

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/crtx"
)

// Consumer subscribes to crtx traffic on the daemon bus and routes
// foreign message-like turns to its handler. With a network adapter
// configured, remote producers' events arrive on the same bus as
// ordinary publishes, so one subscription set covers in-process and
// networked origins alike.
//
// Handlers are synchronous (delivery order matches emission order for
// a single publisher) and always return nil: ingestion is a side
// channel and must never veto a publish (fail-open).
type Consumer struct {
	handler Handler
	acct    *Accounts
	now     func() time.Time
}

// NewConsumer returns a Consumer delivering through h and accounting
// into acct.
func NewConsumer(h Handler, acct *Accounts) *Consumer {
	return &Consumer{handler: h, acct: acct, now: time.Now}
}

// Attach subscribes the consumer and returns a combined unsubscribe.
//
// Patterns are disjoint: canonical turn events (the routable class),
// canonical session lifecycle (accounted only), and vendor extensions
// (accounted only; payloads are caller-defined per events spec §5).
func (c *Consumer) Attach(b bus.Bus) func() {
	subs := []bus.Unsubscribe{
		b.Subscribe("crtx.turn.#", c.onTurn),
		b.Subscribe("crtx.session.#", c.onSession),
		b.Subscribe("x.*.#", c.onExtension),
	}
	return func() {
		for _, unsub := range subs {
			unsub()
		}
	}
}

// turnWire is the transport-agnostic view of a canonical crtx.turn.*
// payload after schema validation: exactly the fields routing needs.
type turnWire struct {
	EnvelopeID      string            `json:"envelope_id"`
	TurnID          string            `json:"turn_id"`
	AgentID         string            `json:"agent_id"`
	InReplyToCallID string            `json:"in_reply_to_call_id"`
	Content         []json.RawMessage `json:"content"`
}

// topicRoles maps the message-like turn topics to the Role their Turn
// carries. Topics absent here (tool.called, tool.completed,
// thinking.emitted) are valid crtx traffic but call mechanics, not
// messages — accounted as skipped.
var topicRoles = map[string]string{
	crtx.TopicUserReceived:     "user",
	crtx.TopicAssistantEmitted: "assistant",
	crtx.TopicSystemInjected:   "system",
}

// onTurn enforces the validation boundary for one canonical turn
// event: self-origin gate, schema validation, then routing policy.
func (c *Consumer) onTurn(_ context.Context, e bus.Event) error {
	if e.Source == crtx.BusSource {
		c.acct.self.Add(1)
		return nil
	}

	topic := string(e.Topic)
	if err := crtx.ValidateEventPayload(topic, e.Payload); err != nil {
		c.acct.invalid.Add(1)
		log.Printf("ingest: drop %s from %q: %v", topic, e.Source, err)
		return nil
	}

	role, ok := topicRoles[topic]
	if !ok {
		c.acct.skipped.Add(1)
		return nil
	}

	var w turnWire
	if err := reencode(e.Payload, &w); err != nil {
		// Unreachable for schema-valid payloads; keep the drop honest.
		c.acct.invalid.Add(1)
		log.Printf("ingest: drop %s from %q: decode: %v", topic, e.Source, err)
		return nil
	}
	if !routableTurn(role, w.InReplyToCallID) {
		c.acct.skipped.Add(1)
		return nil
	}

	at := e.Timestamp
	if at.IsZero() {
		at = c.now()
	}
	c.handler(Message{
		EnvelopeID:      w.EnvelopeID,
		TurnID:          w.TurnID,
		Role:            role,
		AgentID:         w.AgentID,
		InReplyToCallID: w.InReplyToCallID,
		Content:         w.Content,
		Origin: Origin{
			Transport: "bus",
			Topic:     topic,
			BusSource: e.Source,
		},
		ReceivedAt: at.UTC(),
	})
	c.acct.accepted.Add(1)
	return nil
}

// onSession accounts foreign session lifecycle traffic. Correlation
// uses envelope-level linkage instead (consumer ADR §"routing").
func (c *Consumer) onSession(_ context.Context, e bus.Event) error {
	if e.Source == crtx.BusSource {
		c.acct.self.Add(1)
		return nil
	}
	c.acct.session.Add(1)
	return nil
}

// onExtension accounts x.<vendor>.* traffic; extension payloads are
// opaque by spec and never inbox material.
func (c *Consumer) onExtension(_ context.Context, e bus.Event) error {
	if e.Source == crtx.BusSource {
		c.acct.self.Add(1)
		return nil
	}
	if !strings.HasPrefix(string(e.Topic), "x.") {
		return nil
	}
	c.acct.extension.Add(1)
	return nil
}

// reencode round-trips payload through JSON into out, normalizing
// in-process typed structs and cross-process decoded maps into one
// shape (kit/bus Event.Payload type-erasure contract).
func reencode(payload, out any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
