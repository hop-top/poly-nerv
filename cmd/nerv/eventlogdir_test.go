package main

import (
	"path/filepath"
	"testing"
)

// TestEventLogDir pins tail/replay/doctor to the segregated events/
// subtree so they keep working after the eventlog move.
func TestEventLogDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	want := filepath.Join("/custom/state", "nerv", "events")
	if got := eventLogDir(); got != want {
		t.Errorf("eventLogDir() = %q, want %q", got, want)
	}
}
