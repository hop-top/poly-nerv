package dispatch

import (
	"context"
	"sync"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"
)

func TestNewRunner_NilBus(t *testing.T) {
	r := NewRunner()
	if r == nil {
		t.Fatal("NewRunner returned nil")
	}
	if r.Bus() != nil {
		t.Errorf("NewRunner: expected nil bus, got %v", r.Bus())
	}
}

func TestNewRunnerWithBus_NonNil(t *testing.T) {
	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	r := NewRunnerWithBus(b)
	if r == nil {
		t.Fatal("NewRunnerWithBus returned nil")
	}
	if r.Bus() == nil {
		t.Fatal("NewRunnerWithBus: bus is nil; expected non-nil")
	}
	if r.Bus() != b {
		t.Errorf("NewRunnerWithBus: bus reference mismatch")
	}
}

func TestNewRunnerWithBus_NilArg(t *testing.T) {
	// Passing nil is allowed — caller opts out of bus wiring.
	r := NewRunnerWithBus(nil)
	if r == nil {
		t.Fatal("NewRunnerWithBus(nil) returned nil")
	}
	if r.Bus() != nil {
		t.Errorf("NewRunnerWithBus(nil): expected nil bus, got %v", r.Bus())
	}
}

// TestBus_RoundTrip verifies that a publisher and subscriber wired to the
// same bus instance round-trip an event through the memory adapter
// within 10ms. Covers acceptance criterion in US-0016.
func TestBus_RoundTrip(t *testing.T) {
	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	var (
		mu       sync.Mutex
		received []bus.Event
	)
	done := make(chan struct{})

	unsub := b.Subscribe("inbox.message.added", func(_ context.Context, e bus.Event) error {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
		close(done)
		return nil
	})
	defer unsub()

	start := time.Now()
	if err := b.Publish(context.Background(), bus.NewEvent(
		"inbox.message.added",
		"test.publisher",
		map[string]string{"id": "msg-1"},
	)); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Millisecond):
		t.Fatalf("subscriber did not receive event within 10ms (US-0016 AC)")
	}

	elapsed := time.Since(start)
	if elapsed > 10*time.Millisecond {
		t.Errorf("round-trip took %v, expected <=10ms", elapsed)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if string(received[0].Topic) != "inbox.message.added" {
		t.Errorf("unexpected topic: %s", received[0].Topic)
	}
	if received[0].Source != "test.publisher" {
		t.Errorf("unexpected source: %s", received[0].Source)
	}
}

// TestBus_RunnerWired verifies a Runner constructed with a bus can
// publish through its bus reference and another subscriber on the same
// bus receives the event. This proves the dispatch path can publish
// future hook events end-to-end.
func TestBus_RunnerWired(t *testing.T) {
	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	r := NewRunnerWithBus(b)
	if r.Bus() == nil {
		t.Fatal("runner has nil bus")
	}

	gotCh := make(chan bus.Event, 1)
	unsub := b.Subscribe("hook.dispatch.test", func(_ context.Context, e bus.Event) error {
		gotCh <- e
		return nil
	})
	defer unsub()

	// Publish via the runner's bus reference (simulating a future hook
	// emit path).
	if err := r.Bus().Publish(context.Background(), bus.NewEvent(
		"hook.dispatch.test", "nerv.dispatch", "payload",
	)); err != nil {
		t.Fatalf("Publish via runner bus failed: %v", err)
	}

	select {
	case e := <-gotCh:
		if e.Source != "nerv.dispatch" {
			t.Errorf("unexpected source: %s", e.Source)
		}
	case <-time.After(10 * time.Millisecond):
		t.Fatal("subscriber did not receive event within 10ms")
	}
}
