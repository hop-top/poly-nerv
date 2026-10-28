package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/config"
)

// TestInitBus_NonNil verifies initBus returns a non-nil bus and
// populates the package-level daemonBus.
func TestInitBus_NonNil(t *testing.T) {
	// Save and restore daemonBus so this test does not leak state into
	// other tests in the package.
	prev := daemonBus
	defer func() { daemonBus = prev }()
	daemonBus = nil

	b := initBus(config.BusConfig{})
	defer func() { _ = b.Close(context.Background()) }()

	if b == nil {
		t.Fatal("initBus returned nil")
	}
	if Bus() == nil {
		t.Fatal("Bus() returned nil after initBus")
	}
	if Bus() != b {
		t.Errorf("Bus() != initBus() return value")
	}
}

// TestInitBus_PubSubRoundTrip verifies the bus initialized by initBus
// supports publish/subscribe round-trip via the default memory
// adapter within 10ms (US-0016 acceptance criterion).
func TestInitBus_PubSubRoundTrip(t *testing.T) {
	prev := daemonBus
	defer func() { daemonBus = prev }()
	daemonBus = nil

	b := initBus(config.BusConfig{})
	defer func() { _ = b.Close(context.Background()) }()

	var (
		mu       sync.Mutex
		received int
	)
	done := make(chan struct{})

	unsub := b.Subscribe("test.event", func(_ context.Context, _ bus.Event) error {
		mu.Lock()
		received++
		mu.Unlock()
		close(done)
		return nil
	})
	defer unsub()

	if err := b.Publish(context.Background(), bus.NewEvent(
		"test.event", "nervd.test", nil,
	)); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Millisecond):
		t.Fatal("subscriber did not receive event within 10ms")
	}

	mu.Lock()
	defer mu.Unlock()
	if received != 1 {
		t.Errorf("expected 1 event delivered, got %d", received)
	}
}

// TestInitBus_ExactMatchRouting verifies a subscriber for one topic
// does NOT receive an event published on a different topic
// (US-0016 AC: exact-match routing).
func TestInitBus_ExactMatchRouting(t *testing.T) {
	prev := daemonBus
	defer func() { daemonBus = prev }()
	daemonBus = nil

	b := initBus(config.BusConfig{})
	defer func() { _ = b.Close(context.Background()) }()

	var (
		mu     sync.Mutex
		called bool
	)

	unsub := b.Subscribe("inbox.message.added", func(_ context.Context, _ bus.Event) error {
		mu.Lock()
		called = true
		mu.Unlock()
		return nil
	})
	defer unsub()

	if err := b.Publish(context.Background(), bus.NewEvent(
		"some.other.topic", "nervd.test", nil,
	)); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Give any wrong delivery a chance to land before asserting absence.
	time.Sleep(5 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if called {
		t.Error("subscriber received unrelated event; expected exact-match routing only")
	}
}
