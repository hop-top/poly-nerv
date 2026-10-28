package inbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/crtx"
	"hop.top/nerv/internal/ingest"
)

func foreignMsg(envID, turnID string) ingest.Message {
	return ingest.Message{
		EnvelopeID: envID,
		TurnID:     turnID,
		Role:       "user",
		Content: []json.RawMessage{
			json.RawMessage(`{"type":"text","text":"a foreign agent says hello"}`),
		},
		Origin: ingest.Origin{
			Transport: "bus",
			Topic:     crtx.TopicUserReceived,
			BusSource: "stem",
		},
		ReceivedAt: time.Date(2026, 8, 8, 11, 0, 0, 0, time.UTC),
	}
}

func TestRouter_DeliversToInboxWithAttribution(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	r := NewRouter(store, RouterConfig{AgentID: "me"})

	r.Handle(foreignMsg("env-9", "t-9"))

	entries, err := store.List()
	if err != nil || len(entries) != 1 {
		t.Fatalf("List = %v entries (err=%v), want 1", len(entries), err)
	}
	e := entries[0]
	if e.MsgID != MessageID("env-9", "t-9") {
		t.Errorf("msg_id = %q, want deterministic digest %q", e.MsgID, MessageID("env-9", "t-9"))
	}
	if e.InboxID != InboxForeignTurns || e.Kind != KindForeignTurn {
		t.Errorf("container = %q kind %q", e.InboxID, e.Kind)
	}
	if e.EnvelopeID != "env-9" || e.TurnID != "t-9" || e.Role != "user" {
		t.Errorf("identity = %+v", e)
	}
	if e.Origin.BusSource != "stem" || e.Origin.Topic != crtx.TopicUserReceived {
		t.Errorf("origin = %+v", e.Origin)
	}
	if e.Preview != "a foreign agent says hello" {
		t.Errorf("preview = %q", e.Preview)
	}
	if e.SchemaVersion != EntrySchemaVersion {
		t.Errorf("schema_version = %q", e.SchemaVersion)
	}
}

func TestRouter_DedupOnMessageID(t *testing.T) {
	store := NewStore(t.TempDir())
	r := NewRouter(store, RouterConfig{})

	// Same logical turn observed twice (bus + scan overlap, replay).
	r.Handle(foreignMsg("env-9", "t-9"))
	scanForm := foreignMsg("env-9", "t-9")
	scanForm.Origin = ingest.Origin{Transport: "scan", Path: "/x/env-9.jsonl",
		SourceKind: "stem", SourceVersion: "0.1.0"}
	r.Handle(scanForm)

	entries, err := store.List()
	if err != nil || len(entries) != 1 {
		t.Fatalf("List = %d entries (err=%v), want exactly 1", len(entries), err)
	}
	snap := r.Snapshot()
	if snap["delivered"] != 1 || snap["duplicate"] != 1 {
		t.Errorf("router counters = %v, want delivered=1 duplicate=1", snap)
	}
}

func TestRouter_PublishesInboxAddedLifecycleEvent(t *testing.T) {
	store := NewStore(t.TempDir())
	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	var mu sync.Mutex
	var events []bus.Event
	unsub := b.Subscribe(crtx.TopicVendorInboxMessageAdded, func(_ context.Context, e bus.Event) error {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
		return nil
	})
	defer unsub()

	r := NewRouter(store, RouterConfig{Bus: b, AgentID: "me"})
	r.Handle(foreignMsg("env-9", "t-9"))
	r.Handle(foreignMsg("env-9", "t-9")) // duplicate: no re-publish

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 1 {
		t.Fatalf("inbox_message.added published %d times, want 1", len(events))
	}
	e := events[0]
	if e.Source != crtx.BusSource {
		t.Errorf("event source = %q, want %q", e.Source, crtx.BusSource)
	}

	// Payload shape per the spec/events.yaml catalog entry.
	raw, err := json.Marshal(e.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		SchemaVersion   string `json:"schema_version"`
		MsgID           string `json:"msg_id"`
		AgentID         string `json:"agent_id"`
		InboxID         string `json:"inbox_id"`
		SourceSessionID string `json:"source_session_id"`
		Kind            string `json:"kind"`
		TS              string `json:"ts"`
		PayloadRef      string `json:"payload_ref"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("payload decode: %v", err)
	}
	if p.SchemaVersion != "1.0" || p.MsgID != MessageID("env-9", "t-9") ||
		p.AgentID != "me" || p.InboxID != InboxForeignTurns ||
		p.SourceSessionID != "env-9" || p.Kind != KindForeignTurn {
		t.Errorf("payload = %+v", p)
	}
	if p.TS == "" {
		t.Error("payload.ts empty")
	}
	if p.PayloadRef == "" {
		t.Error("payload.payload_ref empty; consumers need the entry path")
	} else if _, err := os.Stat(p.PayloadRef); err != nil {
		t.Errorf("payload_ref %s not readable: %v", p.PayloadRef, err)
	}
}

func TestRouter_CorrelatesLineageToLocalSessions(t *testing.T) {
	sessions := t.TempDir()
	// A local session this daemon produced.
	if err := os.WriteFile(filepath.Join(sessions, "local-sess-7.jsonl"),
		[]byte(`{"crtx_version":"0.1","id":"local-sess-7"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(t.TempDir())
	r := NewRouter(store, RouterConfig{LocalSessionsDir: sessions})

	// Foreign envelope dispatched from the local session.
	m := foreignMsg("child-env", "t-1")
	m.Lineage = ingest.Lineage{
		DispatchedFromEnvelopeID: "local-sess-7",
		DispatchedFromCallID:     "call-3",
	}
	r.Handle(m)

	// Foreign envelope with unrelated lineage: no correlation.
	m2 := foreignMsg("other-env", "t-2")
	m2.Lineage = ingest.Lineage{ParentID: "nobody-home"}
	r.Handle(m2)

	got, err := store.Get(MessageID("child-env", "t-1"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Correlation == nil || got.Correlation.LocalSessionID != "local-sess-7" ||
		got.Correlation.Relation != "dispatched_from" {
		t.Errorf("correlation = %+v", got.Correlation)
	}

	got2, err := store.Get(MessageID("other-env", "t-2"))
	if err != nil {
		t.Fatal(err)
	}
	if got2.Correlation != nil {
		t.Errorf("unrelated lineage correlated: %+v", got2.Correlation)
	}
}

// TestRouter_EndToEndBusDelivery is the payoff check: a foreign turn
// published on the daemon bus lands as a readable inbox entry, and a
// replay of the same event does not deliver twice.
func TestRouter_EndToEndBusDelivery(t *testing.T) {
	store := NewStore(t.TempDir())
	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	r := NewRouter(store, RouterConfig{Bus: b})
	acct := ingest.NewAccounts()
	defer ingest.NewConsumer(r.Handle, acct).Attach(b)()

	payload := map[string]any{
		"envelope_id": "sess-remote",
		"turn_id":     "rt-1",
		"content":     []any{map[string]any{"type": "text", "text": "ping from another agent"}},
	}
	for i := 0; i < 2; i++ { // second publish = replay
		if err := b.Publish(context.Background(),
			bus.NewEvent(bus.Topic(crtx.TopicUserReceived), "stem", payload)); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	entries, err := store.List()
	if err != nil || len(entries) != 1 {
		t.Fatalf("List = %d (err=%v), want 1", len(entries), err)
	}
	if entries[0].Preview != "ping from another agent" {
		t.Errorf("preview = %q", entries[0].Preview)
	}
}

// TestRouter_ConcurrentDelivery exercises the store's per-message
// serialization under -race: many goroutines racing on overlapping
// message ids must deliver each exactly once.
func TestRouter_ConcurrentDelivery(t *testing.T) {
	store := NewStore(t.TempDir())
	r := NewRouter(store, RouterConfig{})

	const workers = 8
	const msgs = 20
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < msgs; i++ {
				r.Handle(foreignMsg("env-c", "t-"+string(rune('a'+i))))
			}
		}()
	}
	wg.Wait()

	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != msgs {
		t.Errorf("delivered %d entries, want %d", len(entries), msgs)
	}
	if got := r.Snapshot()["delivered"]; got != msgs {
		t.Errorf("delivered counter = %d, want %d", got, msgs)
	}
}
