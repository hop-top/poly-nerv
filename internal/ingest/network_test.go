package ingest

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/crtx"
)

// TestConsumer_ReceivesOverNetworkAdapter proves the full fan-in path
// the consumer ADR relies on: a remote producer publishes on its own
// bus, the kit/bus network adapter carries the event over WebSocket
// into the local daemon bus, and the consumer routes it — while the
// local producer's own emissions on the same bus stay filtered.
func TestConsumer_ReceivesOverNetworkAdapter(t *testing.T) {
	// Local daemon bus with a listening network adapter.
	local := bus.New()
	adapterLocal := bus.NewNetworkAdapter(local, bus.WithOriginID("local"))
	srv := httptest.NewServer(adapterLocal.Handler())
	defer srv.Close()
	defer func() { _ = adapterLocal.Close() }()

	col := &collector{}
	acct := NewAccounts()
	defer NewConsumer(col.handle, acct).Attach(local)()

	// Remote producer bus dials in.
	remote := bus.New()
	adapterRemote := bus.NewNetworkAdapter(remote, bus.WithOriginID("remote"))
	defer func() { _ = adapterRemote.Close() }()
	wsURL := "ws" + srv.URL[4:]
	if err := adapterRemote.Connect(context.Background(), wsURL); err != nil {
		t.Fatalf("connect: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	// Remote foreign turn.
	if err := remote.Publish(context.Background(), bus.NewEvent(
		bus.Topic(crtx.TopicUserReceived), "stem", map[string]any{
			"envelope_id": "net-env-1",
			"turn_id":     "nt-1",
			"content":     []any{map[string]any{"type": "text", "text": "over the wire"}},
		})); err != nil {
		t.Fatalf("remote publish: %v", err)
	}

	// Local own emission on the same bus: must stay filtered.
	if err := local.Publish(context.Background(), bus.NewEvent(
		bus.Topic(crtx.TopicUserReceived), crtx.BusSource, map[string]any{
			"envelope_id": "own-env-1",
			"turn_id":     "ot-1",
			"content":     []any{map[string]any{"type": "text", "text": "own"}},
		})); err != nil {
		t.Fatalf("local publish: %v", err)
	}

	deadline := time.After(3 * time.Second)
	for len(col.all()) < 1 {
		select {
		case <-deadline:
			t.Fatalf("network event never delivered; accounts=%v", acct.Snapshot())
		case <-time.After(5 * time.Millisecond):
		}
	}

	msgs := col.all()
	if len(msgs) != 1 {
		t.Fatalf("delivered %d, want exactly 1 (own emission filtered): %+v", len(msgs), msgs)
	}
	if msgs[0].EnvelopeID != "net-env-1" || msgs[0].Origin.BusSource != "stem" {
		t.Errorf("delivered = %+v", msgs[0])
	}
	snap := acct.Snapshot()
	if snap["self"] != 1 {
		t.Errorf("self = %d, want 1", snap["self"])
	}
}
