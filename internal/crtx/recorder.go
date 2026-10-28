package crtx

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"hop.top/kit/go/runtime/bus"
)

// Recorder subscribes to crtx topics on a bus and folds event
// payloads into envelope JSONL via a SessionStore. It needs no
// translation layer: canonical payloads ARE ContentParts / Envelope
// metadata fragments by the events-spec invariant.
//
// Handlers are synchronous so the persisted Turn order matches the
// canonical emission order, and always return nil: recording is a
// side channel and must never fail the publisher (fail-open).
type Recorder struct {
	store *SessionStore
	now   func() time.Time
}

// NewRecorder returns a Recorder appending through store.
func NewRecorder(store *SessionStore) *Recorder {
	return &Recorder{store: store, now: time.Now}
}

// Attach subscribes the recorder to every topic it folds and returns
// a combined unsubscribe.
//
// Only this producer's own emissions fold (Event.Source == BusSource):
// with a network adapter on the bus, foreign producers' crtx events
// arrive as ordinary publishes, and folding them would duplicate the
// foreign transcript under the local sessions scan root. The consumer
// path owns foreign events (docs/architecture/crtx-consumer.md).
func (r *Recorder) Attach(b bus.Bus) func() {
	subs := []bus.Unsubscribe{
		b.Subscribe(TopicSessionCreated, ownSource(r.onSessionCreated)),
		b.Subscribe(TopicUserReceived, ownSource(r.turnHandler("user", contentVerbatim))),
		b.Subscribe(TopicAssistantEmitted, ownSource(r.turnHandler("assistant", contentVerbatim))),
		b.Subscribe(TopicSystemInjected, ownSource(r.turnHandler("system", contentVerbatim))),
		b.Subscribe(TopicThinkingEmitted, ownSource(r.turnHandler("assistant", contentThinking))),
		b.Subscribe(TopicToolCalled, ownSource(r.turnHandler("assistant", contentToolCall))),
		b.Subscribe(TopicToolCompleted, ownSource(r.turnHandler("tool", contentToolResult))),
		b.Subscribe(TopicVendorSessionEnded, ownSource(r.onSessionEnded)),
	}
	return func() {
		for _, unsub := range subs {
			unsub()
		}
	}
}

// ownSource gates a handler to events this producer published itself.
func ownSource(h bus.Handler) bus.Handler {
	return func(ctx context.Context, e bus.Event) error {
		if e.Source != BusSource {
			return nil
		}
		return h(ctx, e)
	}
}

// wireTurnEvent is the transport-agnostic view of any crtx.turn.*
// payload: identity fields plus everything needed to rebuild the
// ContentParts for the Turn. Decoded via a JSON round-trip so
// in-process structs and cross-process map forms behave identically.
type wireTurnEvent struct {
	EnvelopeID      string            `json:"envelope_id"`
	TurnID          string            `json:"turn_id"`
	Content         []json.RawMessage `json:"content"`
	AgentID         string            `json:"agent_id"`
	InReplyToCallID string            `json:"in_reply_to_call_id"`

	// tool.called / tool.completed convenience-extraction fields.
	CallID          string          `json:"call_id"`
	Name            string          `json:"name"`
	Input           json.RawMessage `json:"input"`
	ParentCallID    string          `json:"parent_call_id"`
	Output          json.RawMessage `json:"output"`
	IsError         bool            `json:"is_error"`
	ChildEnvelopeID string          `json:"child_envelope_id"`

	// thinking.emitted fields.
	Text      string `json:"text"`
	Signature string `json:"signature"`
}

// contentFunc rebuilds a Turn's content array from the wire event.
type contentFunc func(w *wireTurnEvent) ([]json.RawMessage, error)

// contentVerbatim appends the payload's content array unchanged
// (user.received, assistant.emitted, system.injected).
func contentVerbatim(w *wireTurnEvent) ([]json.RawMessage, error) {
	if len(w.Content) == 0 {
		return nil, fmt.Errorf("payload carries no content parts")
	}
	return w.Content, nil
}

// contentToolCall rebuilds the tool_call ContentPart from the
// tool.called convenience-extraction fields (spec: field semantics
// identical to ToolCallPart).
func contentToolCall(w *wireTurnEvent) ([]json.RawMessage, error) {
	if w.CallID == "" {
		return nil, fmt.Errorf("tool.called payload missing call_id")
	}
	input := any(map[string]any{})
	if len(w.Input) > 0 {
		var v any
		if err := json.Unmarshal(w.Input, &v); err == nil && v != nil {
			input = v
		}
	}
	return marshalPart(ToolCallPart{
		Type:         "tool_call",
		CallID:       w.CallID,
		Name:         w.Name,
		Input:        input,
		ParentCallID: w.ParentCallID,
	})
}

// contentToolResult rebuilds the tool_result ContentPart from the
// tool.completed fields.
func contentToolResult(w *wireTurnEvent) ([]json.RawMessage, error) {
	if w.CallID == "" {
		return nil, fmt.Errorf("tool.completed payload missing call_id")
	}
	var output any
	if len(w.Output) > 0 {
		_ = json.Unmarshal(w.Output, &output)
	}
	return marshalPart(ToolResultPart{
		Type:            "tool_result",
		CallID:          w.CallID,
		Output:          output,
		IsError:         w.IsError,
		ChildEnvelopeID: w.ChildEnvelopeID,
	})
}

// contentThinking rebuilds the thinking ContentPart from the
// thinking.emitted fields.
func contentThinking(w *wireTurnEvent) ([]json.RawMessage, error) {
	part := map[string]any{"type": "thinking", "text": w.Text}
	if w.Signature != "" {
		part["signature"] = w.Signature
	}
	return marshalPart(part)
}

func marshalPart(part any) ([]json.RawMessage, error) {
	raw, err := json.Marshal(part)
	if err != nil {
		return nil, err
	}
	return []json.RawMessage{raw}, nil
}

// turnHandler folds one crtx.turn.* topic into an appended Turn with
// the given role.
func (r *Recorder) turnHandler(role string, content contentFunc) bus.Handler {
	return func(_ context.Context, e bus.Event) error {
		var w wireTurnEvent
		if err := reencode(e.Payload, &w); err != nil {
			log.Printf("crtx: recorder %s: bad payload (fail-open): %v", e.Topic, err)
			return nil
		}
		if w.EnvelopeID == "" {
			log.Printf("crtx: recorder %s: missing envelope_id (fail-open)", e.Topic)
			return nil
		}
		parts, err := content(&w)
		if err != nil {
			log.Printf("crtx: recorder %s: %v (fail-open)", e.Topic, err)
			return nil
		}

		at := e.Timestamp
		if at.IsZero() {
			at = r.now()
		}
		turn := Turn{
			ID:              w.TurnID,
			Role:            role,
			CreatedAt:       at.UTC(),
			Content:         parts,
			AgentID:         w.AgentID,
			InReplyToCallID: w.InReplyToCallID,
		}
		if turn.ID == "" {
			turn.ID = fmt.Sprintf("t-%d", at.UTC().UnixNano())
		}
		if err := r.store.AppendTurn(w.EnvelopeID, turn); err != nil {
			log.Printf("crtx: recorder %s: append %s (fail-open): %v", e.Topic, w.EnvelopeID, err)
		}
		return nil
	}
}

// onSessionCreated materializes the envelope file.
func (r *Recorder) onSessionCreated(_ context.Context, e bus.Event) error {
	var w struct {
		EnvelopeID string `json:"envelope_id"`
		CreatedAt  string `json:"created_at"`
		Source     Source `json:"source"`
	}
	if err := reencode(e.Payload, &w); err != nil || w.EnvelopeID == "" {
		log.Printf("crtx: recorder %s: bad payload (fail-open): %v", e.Topic, err)
		return nil
	}
	createdAt, err := time.Parse(time.RFC3339Nano, w.CreatedAt)
	if err != nil {
		createdAt = e.Timestamp
	}
	if w.Source.Kind == "" {
		w.Source.Kind = "custom"
	}
	if w.Source.Version == "" {
		w.Source.Version = "unknown"
	}
	if err := r.store.Create(w.EnvelopeID, createdAt, w.Source); err != nil {
		log.Printf("crtx: recorder %s: create %s (fail-open): %v", e.Topic, w.EnvelopeID, err)
	}
	return nil
}

// onSessionEnded records the terminating hook payload as envelope
// metadata (crtx v0.1 has no lifecycle terminator topic).
func (r *Recorder) onSessionEnded(_ context.Context, e bus.Event) error {
	var w struct {
		EnvelopeID string          `json:"envelope_id"`
		OccurredAt string          `json:"occurred_at"`
		Payload    json.RawMessage `json:"payload"`
	}
	if err := reencode(e.Payload, &w); err != nil || w.EnvelopeID == "" {
		log.Printf("crtx: recorder %s: bad payload (fail-open): %v", e.Topic, err)
		return nil
	}
	value := map[string]any{"occurred_at": w.OccurredAt}
	if len(w.Payload) > 0 {
		var detail any
		if err := json.Unmarshal(w.Payload, &detail); err == nil {
			value["payload"] = detail
		}
	}
	if err := r.store.SetMetadata(w.EnvelopeID, MetaSessionEnd, value); err != nil {
		log.Printf("crtx: recorder %s: metadata %s (fail-open): %v", e.Topic, w.EnvelopeID, err)
	}
	return nil
}

// reencode round-trips payload through JSON into out, normalizing
// in-process typed structs and cross-process decoded maps into one
// shape (see kit/bus Event.Payload type-erasure contract).
func reencode(payload, out any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
