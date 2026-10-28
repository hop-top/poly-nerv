package dispatch

import (
	"testing"
	"time"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/eventlog"
	"hop.top/nerv/internal/merger"
)

// mockLogger records Write calls for assertion.
type mockLogger struct {
	entries []writeCall
}

type writeCall struct {
	env      *envelope.Envelope
	decision interface{}
}

func (m *mockLogger) Write(env *envelope.Envelope, decision interface{}) error {
	m.entries = append(m.entries, writeCall{env: env, decision: decision})
	return nil
}

func (m *mockLogger) Query(_ eventlog.Filter) ([]eventlog.Entry, error) {
	return nil, nil
}

func newTestEnvelope(event string) *envelope.Envelope {
	return &envelope.Envelope{
		NervVersion: "1.0",
		Event:       event,
		EventID:     "test-001",
		Timestamp:   time.Now(),
		CLI:         envelope.CLIInfo{Name: "test-cli", Version: "0.1.0"},
		Session:     envelope.SessionInfo{ID: "sess-001"},
	}
}

func TestDispatch_NoHooks(t *testing.T) {
	d := New(compileCfg(), nil, NewMatcher(), NewRunner())
	res, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision.Action != merger.Allow {
		t.Errorf("want allow, got %s", res.Decision.Action)
	}
}

func TestDispatch_EventLookup(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "lint", On: "PreToolUse", Run: "/bin/lint"},
		config.HookEntry{Name: "audit", On: "PostToolUse", Run: "/bin/audit"},
	)
	d := New(cfg, nil, NewMatcher(), NewRunner())

	res, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision.Action != merger.Allow {
		t.Errorf("want allow, got %s", res.Decision.Action)
	}
}

func TestDispatch_UnknownEvent(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "lint", On: "PreToolUse", Run: "/bin/lint"},
	)
	d := New(cfg, nil, NewMatcher(), NewRunner())

	res, err := d.Dispatch(newTestEnvelope("UnknownEvent"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision.Action != merger.Allow {
		t.Errorf("want allow for unknown event, got %s", res.Decision.Action)
	}
}

func TestDispatch_TimingsPopulated(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "lint", On: "PreToolUse", Run: "/bin/lint"},
	)
	lg := &mockLogger{}
	d := New(cfg, lg, NewMatcher(), NewRunner())

	res, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Timings.TotalMs < 0 {
		t.Errorf("TotalMs should be >= 0, got %d", res.Timings.TotalMs)
	}
	if res.Timings.MatchMs < 0 {
		t.Errorf("MatchMs should be >= 0, got %d", res.Timings.MatchMs)
	}
	if res.Timings.MergeMs < 0 {
		t.Errorf("MergeMs should be >= 0, got %d", res.Timings.MergeMs)
	}
}

func TestDispatch_EventLogged(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "lint", On: "PreToolUse", Run: "/bin/lint"},
	)
	lg := &mockLogger{}
	d := New(cfg, lg, NewMatcher(), NewRunner())

	env := newTestEnvelope("PreToolUse")
	_, err := d.Dispatch(env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lg.entries) != 1 {
		t.Fatalf("want 1 log entry, got %d", len(lg.entries))
	}
	if lg.entries[0].env != env {
		t.Error("logged envelope does not match input")
	}
	dec, ok := lg.entries[0].decision.(merger.Decision)
	if !ok {
		t.Fatalf("logged decision is not merger.Decision: %T",
			lg.entries[0].decision)
	}
	if dec.Action != merger.Allow {
		t.Errorf("logged decision: want allow, got %s", dec.Action)
	}
}
