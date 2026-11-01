//go:build !windows

package dispatch

import (
	"errors"
	"testing"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/eventlog"
	"hop.top/nerv/internal/merger"
)

type failLogger struct{}

func (failLogger) Write(_ *envelope.Envelope, _ interface{}) error { return errors.New("disk full") }
func (failLogger) Query(eventlog.Filter) ([]eventlog.Entry, error) {
	return nil, nil
}

// An event-log write failure is fail-open for the log only: the merged
// decision (here a block) must still be returned.
func TestDispatch_LogFailureKeepsDecision(t *testing.T) {
	b := writeScript(t, t.TempDir(), "b.sh",
		"#!/bin/sh\necho '{\"action\":\"block\",\"message\":\"no\"}'\nexit 2\n")
	d := New(compileCfg(config.HookEntry{Name: "b", On: "PreToolUse", Run: b}),
		failLogger{}, NewMatcher(), NewRunner())
	res, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil || res == nil {
		t.Fatalf("Dispatch = %v, %v; want result, nil", res, err)
	}
	if res.Decision.Action != merger.Block {
		t.Fatalf("action = %s, want block", res.Decision.Action)
	}
}
