package daemon

import (
	"context"
	"testing"

	nervv1 "hop.top/nerv/gen/go/nerv/v1"
	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/dispatch"
	"hop.top/nerv/internal/events"
)

// TestHandler_Dispatch_ReturnsDecision asserts the Handler routes a
// proto DispatchRequest through internal/dispatch and returns a proto
// DispatchResponse whose Decision.Action is ACTION_ALLOW for the
// no-rules baseline (empty CachedConfig → no hooks match → merger
// returns Allow per spec).
//
// TDD red: NewHandler/Handler.Dispatch are stubbed in handler.go and
// return "not implemented" — this test fails until the handler lands.
func TestHandler_Dispatch_ReturnsDecision(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	req := &nervv1.DispatchRequest{
		Envelope: &nervv1.Envelope{
			NervVersion: "1.0.0-draft",
			Event:       string(events.EventPreToolUse),
			EventId:     "test-event-id",
			Cli: &nervv1.CLIInfo{
				Name:    "test-cli",
				Version: "0.0.0",
			},
			Session: &nervv1.SessionInfo{
				Id:  "test-session",
				Cwd: "/tmp",
			},
		},
	}

	resp, err := h.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if resp == nil {
		t.Fatal("Dispatch: nil response")
	}
	if resp.GetDecision() == nil {
		t.Fatal("Dispatch: nil decision")
	}
	if got, want := resp.GetDecision().GetAction(), nervv1.Action_ACTION_ALLOW; got != want {
		t.Fatalf("Decision.Action = %v, want %v", got, want)
	}
}

// TestHandler_List_ReturnsAllEvents asserts the Handler returns every
// event from internal/events as a proto EventDescriptor. Count is
// pinned to len(events.AllEvents()) so the test stays correct when
// codegen adds or removes events.
func TestHandler_List_ReturnsAllEvents(t *testing.T) {
	t.Parallel()

	h := newTestHandler(t)

	resp, err := h.List(context.Background(), &nervv1.ListRequest{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if resp == nil {
		t.Fatal("List: nil response")
	}

	want := len(events.AllEvents())
	if got := len(resp.GetEvents()); got != want {
		t.Fatalf("List returned %d events, want %d", got, want)
	}

	// Spot-check: the first descriptor maps to the first AllEvents
	// entry with category + origin + blocking populated.
	first := resp.GetEvents()[0]
	wantName := string(events.AllEvents()[0])
	if first.GetName() != wantName {
		t.Errorf("first event name = %q, want %q", first.GetName(), wantName)
	}
	if first.GetCategory() == "" {
		t.Errorf("first event category empty for %q", wantName)
	}
	if first.GetOrigin() == "" {
		t.Errorf("first event origin empty for %q", wantName)
	}
}

// newTestHandler builds a Handler over a Dispatcher with empty config
// (no hooks). The merger returns Allow when no decisions are produced,
// so this is the canonical "no rules" baseline.
func newTestHandler(t *testing.T) *Handler {
	t.Helper()

	cfg := config.Compile(&config.HooksConfig{})
	matcher := dispatch.NewMatcher()
	runner := dispatch.NewRunner()
	d := dispatch.New(cfg, nil, matcher, runner)

	h, err := NewHandler(d)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}
