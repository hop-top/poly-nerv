//go:build !windows

package main

import (
	"strings"
	"testing"
)

// TestReplayCmd_FlagsReachRunE locks in the fix for the viper-binding
// defect on `nerv replay`: replayCmd registers --last as a local cobra
// flag but it was previously read via the package-global viper
// instance, which kit never binds to. The result: every
// `nerv replay --last 10` ran with lastN=0 and hit the "specify an
// event-id or use --last N" guard.
//
// Regression: read --last off cmd.Flags() instead of viper.
func TestReplayCmd_FlagsReachRunE(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate from any user state/event log
	chdirTemp(t)                  // isolate from project hooks.yaml

	cmd := replayCmd()
	if err := cmd.Flags().Set("last", "10"); err != nil {
		t.Fatalf("set --last: %v", err)
	}

	// With the fix in place, runReplay reads --last off cmd.Flags()
	// and proceeds past the "specify an event-id or use --last N"
	// guard. With the defect, this returns that exact error.
	err := runReplay(cmd, nil)
	if err != nil && strings.Contains(err.Error(), "specify an event-id or use --last N") {
		t.Fatalf("flag --last did not reach RunE: %v", err)
	}
	// Other errors (e.g. event log absent in tmp HOME) are tolerable —
	// we only care that the guard didn't trip.
}

// TestReplayCmd_NoFlagStillErrors confirms the guard still fires when
// neither --last nor a positional event-id is supplied.
func TestReplayCmd_NoFlagStillErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	chdirTemp(t)

	cmd := replayCmd()

	err := runReplay(cmd, nil)
	if err == nil {
		t.Fatal("expected error when neither --last nor event-id is set")
	}
	if !strings.Contains(err.Error(), "specify an event-id or use --last N") {
		t.Fatalf("expected 'specify an event-id or use --last N', got: %v", err)
	}
}

// TestReplayCmd_LastDefault confirms --last has a zero default so the
// guard fires when the user supplies neither --last nor an event-id.
func TestReplayCmd_LastDefault(t *testing.T) {
	cmd := replayCmd()
	last, _ := cmd.Flags().GetInt("last")
	if last != 0 {
		t.Errorf("default --last = %d, want 0", last)
	}
}
