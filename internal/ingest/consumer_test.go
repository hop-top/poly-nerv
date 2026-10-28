package ingest

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/crtx"
)

// collector is a threadsafe Handler capturing delivered messages.
type collector struct {
	mu   sync.Mutex
	msgs []Message
}

func (c *collector) handle(m Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
}

func (c *collector) all() []Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Message, len(c.msgs))
	copy(out, c.msgs)
	return out
}

func newAttached(t *testing.T) (bus.Bus, *collector, *Accounts) {
	t.Helper()
	b := bus.New()
	t.Cleanup(func() { _ = b.Close(context.Background()) })

	col := &collector{}
	acct := NewAccounts()
	unsub := NewConsumer(col.handle, acct).Attach(b)
	t.Cleanup(unsub)
	return b, col, acct
}

func publish(t *testing.T, b bus.Bus, topic, source string, payload any) {
	t.Helper()
	if err := b.Publish(context.Background(),
		bus.NewEvent(bus.Topic(topic), source, payload)); err != nil {
		t.Fatalf("publish %s: %v", topic, err)
	}
}

// mapPayload simulates the cross-process form a network-adapter
// injected event arrives in: JSON-decoded map, not a typed struct.
func mapPayload(t *testing.T, doc string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatalf("bad test payload: %v", err)
	}
	return m
}

func TestConsumer_RoutesForeignUserTurn(t *testing.T) {
	b, col, acct := newAttached(t)

	publish(t, b, crtx.TopicUserReceived, "stem", mapPayload(t, `{
		"envelope_id": "env-f1",
		"turn_id": "t-1",
		"content": [{"type":"text","text":"hello from stem"}]
	}`))

	msgs := col.all()
	if len(msgs) != 1 {
		t.Fatalf("delivered %d messages, want 1", len(msgs))
	}
	m := msgs[0]
	if m.EnvelopeID != "env-f1" || m.TurnID != "t-1" || m.Role != "user" {
		t.Errorf("message identity = %q/%q role %q", m.EnvelopeID, m.TurnID, m.Role)
	}
	if len(m.Content) != 1 {
		t.Fatalf("content parts = %d, want 1", len(m.Content))
	}
	var part struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(m.Content[0], &part); err != nil || part.Text != "hello from stem" {
		t.Errorf("content not preserved verbatim: %s (err=%v)", m.Content[0], err)
	}
	if m.Origin.Transport != "bus" || m.Origin.Topic != crtx.TopicUserReceived || m.Origin.BusSource != "stem" {
		t.Errorf("origin = %+v", m.Origin)
	}
	if got := acct.Snapshot()["accepted"]; got != 1 {
		t.Errorf("accepted = %d, want 1", got)
	}
}

func TestConsumer_RoutesAssistantTurnWithAgentID(t *testing.T) {
	b, col, _ := newAttached(t)

	publish(t, b, crtx.TopicAssistantEmitted, "vein", crtx.AssistantEmittedPayload{
		EnvelopeID: "env-f2",
		TurnID:     "t-2",
		AgentID:    "helper",
		Content:    []any{crtx.NewTextPart("done")},
	})

	msgs := col.all()
	if len(msgs) != 1 {
		t.Fatalf("delivered %d messages, want 1", len(msgs))
	}
	if msgs[0].Role != "assistant" || msgs[0].AgentID != "helper" {
		t.Errorf("role/agent = %q/%q", msgs[0].Role, msgs[0].AgentID)
	}
}

func TestConsumer_SelfOriginFiltered(t *testing.T) {
	b, col, acct := newAttached(t)

	publish(t, b, crtx.TopicUserReceived, crtx.BusSource, mapPayload(t, `{
		"envelope_id": "env-own",
		"turn_id": "t-1",
		"content": [{"type":"text","text":"own emission"}]
	}`))

	if got := col.all(); len(got) != 0 {
		t.Fatalf("own emission delivered: %+v", got)
	}
	snap := acct.Snapshot()
	if snap["self"] != 1 {
		t.Errorf("self = %d, want 1", snap["self"])
	}
	if snap["accepted"] != 0 {
		t.Errorf("accepted = %d, want 0", snap["accepted"])
	}
}

func TestConsumer_InvalidPayloadDroppedWithAccount(t *testing.T) {
	b, col, acct := newAttached(t)

	// Missing envelope_id: fails UserReceivedPayload schema.
	publish(t, b, crtx.TopicUserReceived, "stem", mapPayload(t, `{
		"turn_id": "t-1",
		"content": [{"type":"text","text":"x"}]
	}`))
	// content wrong shape.
	publish(t, b, crtx.TopicAssistantEmitted, "stem", mapPayload(t, `{
		"envelope_id": "e", "turn_id": "t", "content": "nope"
	}`))
	// Payload not even an object.
	publish(t, b, crtx.TopicUserReceived, "stem", "garbage")

	if got := col.all(); len(got) != 0 {
		t.Fatalf("invalid payloads delivered: %+v", got)
	}
	if got := acct.Snapshot()["invalid"]; got != 3 {
		t.Errorf("invalid = %d, want 3", got)
	}
}

func TestConsumer_ExtensionAndSessionAccountedNotRouted(t *testing.T) {
	b, col, acct := newAttached(t)

	publish(t, b, "x.stem.custom.event", "stem", map[string]any{"anything": true})
	publish(t, b, "x.nerv.inbox_message.added", "stem", map[string]any{"msg_id": "m1"})
	publish(t, b, crtx.TopicSessionCreated, "stem", mapPayload(t, `{
		"envelope_id": "env-f3",
		"created_at": "2026-08-08T10:00:00Z",
		"source": {"kind":"stem","version":"0.1.0"}
	}`))

	if got := col.all(); len(got) != 0 {
		t.Fatalf("non-message events delivered: %+v", got)
	}
	snap := acct.Snapshot()
	if snap["extension"] != 2 {
		t.Errorf("extension = %d, want 2", snap["extension"])
	}
	if snap["session"] != 1 {
		t.Errorf("session = %d, want 1", snap["session"])
	}
}

func TestConsumer_MechanicsSkipped(t *testing.T) {
	b, col, acct := newAttached(t)

	publish(t, b, crtx.TopicToolCalled, "stem", mapPayload(t, `{
		"envelope_id":"e","turn_id":"t","call_id":"c1","name":"Bash",
		"input":{"command":"ls"}
	}`))
	publish(t, b, crtx.TopicToolCompleted, "stem", mapPayload(t, `{
		"envelope_id":"e","turn_id":"t2","call_id":"c1","output":"main.go"
	}`))
	publish(t, b, crtx.TopicThinkingEmitted, "stem", mapPayload(t, `{
		"envelope_id":"e","turn_id":"t3","text":"hmm"
	}`))

	if got := col.all(); len(got) != 0 {
		t.Fatalf("mechanics delivered: %+v", got)
	}
	if got := acct.Snapshot()["skipped"]; got != 3 {
		t.Errorf("skipped = %d, want 3", got)
	}
}

func TestConsumer_SystemInjectedOnlyMidFlight(t *testing.T) {
	b, col, _ := newAttached(t)

	// Plain system prompt: transcript context, not a message.
	publish(t, b, crtx.TopicSystemInjected, "stem", mapPayload(t, `{
		"envelope_id":"e","turn_id":"t1",
		"content":[{"type":"text","text":"You are helpful."}]
	}`))
	// Mid-flight interjection targeting an open call: routed.
	publish(t, b, crtx.TopicSystemInjected, "stem", mapPayload(t, `{
		"envelope_id":"e","turn_id":"t2","in_reply_to_call_id":"c9",
		"content":[{"type":"text","text":"stop that tool"}]
	}`))

	msgs := col.all()
	if len(msgs) != 1 {
		t.Fatalf("delivered %d messages, want 1", len(msgs))
	}
	if msgs[0].TurnID != "t2" || msgs[0].InReplyToCallID != "c9" || msgs[0].Role != "system" {
		t.Errorf("routed message = %+v", msgs[0])
	}
}

// TestConsumer_ConcurrentPublish exercises the ingest path under
// concurrency (run with -race): parallel foreign publishes across
// topics must neither race nor lose accounting.
func TestConsumer_ConcurrentPublish(t *testing.T) {
	b, col, acct := newAttached(t)

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			publish(t, b, crtx.TopicUserReceived, "stem", map[string]any{
				"envelope_id": "env-conc",
				"turn_id":     "t",
				"content":     []any{map[string]any{"type": "text", "text": "m"}},
			})
			publish(t, b, crtx.TopicThinkingEmitted, "stem", map[string]any{
				"envelope_id": "env-conc", "turn_id": "t", "text": "…",
			})
		}(i)
	}
	wg.Wait()

	if got := len(col.all()); got != n {
		t.Errorf("delivered %d, want %d", got, n)
	}
	snap := acct.Snapshot()
	if snap["accepted"] != n || snap["skipped"] != n {
		t.Errorf("accepted=%d skipped=%d, want %d/%d", snap["accepted"], snap["skipped"], n, n)
	}
}
