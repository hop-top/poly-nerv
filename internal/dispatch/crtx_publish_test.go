package dispatch

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/crtx"
	"hop.top/nerv/internal/merger"
)

// TestDispatch_PublishesCrtxEmission verifies the dispatcher publishes
// the translated crtx event on the runner's bus after a dispatch.
func TestDispatch_PublishesCrtxEmission(t *testing.T) {
	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	got := make(chan bus.Event, 1)
	unsub := b.Subscribe(crtx.TopicUserReceived, func(_ context.Context, e bus.Event) error {
		got <- e
		return nil
	})
	defer unsub()

	d := New(compileCfg(), nil, NewMatcher(), NewRunnerWithBus(b))

	env := newTestEnvelope("UserPromptSubmit")
	env.Payload = json.RawMessage(`{"prompt":"hello"}`)

	res, err := d.Dispatch(env)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res.Decision.Action != merger.Allow {
		t.Fatalf("want allow, got %s", res.Decision.Action)
	}

	select {
	case e := <-got:
		if string(e.Topic) != crtx.TopicUserReceived {
			t.Errorf("topic = %q, want %q", e.Topic, crtx.TopicUserReceived)
		}
		if e.Source != crtx.BusSource {
			t.Errorf("source = %q, want %q", e.Source, crtx.BusSource)
		}
		p, ok := e.Payload.(crtx.UserReceivedPayload)
		if !ok {
			t.Fatalf("payload type = %T, want crtx.UserReceivedPayload", e.Payload)
		}
		if p.EnvelopeID != env.Session.ID {
			t.Errorf("envelope_id = %q, want %q", p.EnvelopeID, env.Session.ID)
		}
		if p.TurnID != env.EventID {
			t.Errorf("turn_id = %q, want %q", p.TurnID, env.EventID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("no crtx.turn.user.received event published within 100ms")
	}
}

// TestDispatch_NoBusStillDispatches locks the fail-open contract: a
// dispatcher whose runner has no bus dispatches normally and never
// panics on the publish path.
func TestDispatch_NoBusStillDispatches(t *testing.T) {
	d := New(compileCfg(), nil, NewMatcher(), NewRunner())

	env := newTestEnvelope("UserPromptSubmit")
	env.Payload = json.RawMessage(`{"prompt":"hello"}`)

	res, err := d.Dispatch(env)
	if err != nil {
		t.Fatalf("Dispatch without bus: %v", err)
	}
	if res.Decision.Action != merger.Allow {
		t.Errorf("want allow, got %s", res.Decision.Action)
	}
}

// TestDispatch_DroppedEventPublishesNothing verifies events outside
// the ADR mapping table emit nothing on either namespace.
func TestDispatch_DroppedEventPublishesNothing(t *testing.T) {
	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	got := make(chan bus.Event, 4)
	for _, pattern := range []string{"crtx.#", "x.#"} {
		unsub := b.Subscribe(pattern, func(_ context.Context, e bus.Event) error {
			got <- e
			return nil
		})
		defer unsub()
	}

	d := New(compileCfg(), nil, NewMatcher(), NewRunnerWithBus(b))

	env := newTestEnvelope("FileChanged")
	env.Payload = json.RawMessage(`{"file_path":"/tmp/x","change_type":"modify"}`)

	if _, err := d.Dispatch(env); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	select {
	case e := <-got:
		t.Fatalf("dropped event published %q", e.Topic)
	case <-time.After(20 * time.Millisecond):
	}
}

// TestDispatch_ClosedBusFailOpen verifies a publish failure (bus
// already closed) never fails the dispatch.
func TestDispatch_ClosedBusFailOpen(t *testing.T) {
	b := bus.New()
	_ = b.Close(context.Background())

	d := New(compileCfg(), nil, NewMatcher(), NewRunnerWithBus(b))

	env := newTestEnvelope("UserPromptSubmit")
	env.Payload = json.RawMessage(`{"prompt":"hello"}`)

	res, err := d.Dispatch(env)
	if err != nil {
		t.Fatalf("Dispatch with closed bus: %v", err)
	}
	if res.Decision.Action != merger.Allow {
		t.Errorf("want allow, got %s", res.Decision.Action)
	}
}
