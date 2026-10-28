package vibe

import (
	"strings"
	"testing"

	"hop.top/nerv/internal/events"
)

// --- Issue 6: PreSession/PostSession capability claims ---

func TestRegression_PreSession_RequiresSessionStartMapping(t *testing.T) {
	a := &Adapter{}
	caps := a.Capabilities()
	if !caps.PreSession {
		t.Skip("PreSession=false; no mapping required")
	}

	// If PreSession is true, vibeToNerv must map something to SessionStart.
	found := false
	for _, nerv := range vibeToNerv {
		if nerv == events.EventSessionStart {
			found = true
			break
		}
	}
	if !found {
		t.Error(
			"Capabilities().PreSession=true but vibeToNerv has " +
				"no mapping producing SessionStart",
		)
	}
}

func TestRegression_PostSession_RequiresSessionEndMapping(t *testing.T) {
	a := &Adapter{}
	caps := a.Capabilities()
	if !caps.PostSession {
		t.Skip("PostSession=false; no mapping required")
	}

	// If PostSession is true, vibeToNerv must map something to SessionEnd.
	found := false
	for _, nerv := range vibeToNerv {
		if nerv == events.EventSessionEnd {
			found = true
			break
		}
	}
	if !found {
		t.Error(
			"Capabilities().PostSession=true but vibeToNerv has " +
				"no mapping producing SessionEnd",
		)
	}
}

// --- Issue 8: MergeConfig TOML preservation ---

func TestRegression_MergeConfig_PreservesExistingTOML(t *testing.T) {
	a := &Adapter{}
	existing := []byte("[general]\ntheme = \"dark\"\n")
	merged, err := a.MergeConfig(existing)
	if err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}
	if !strings.Contains(string(merged), `theme = "dark"`) {
		t.Error(
			"MergeConfig did not preserve existing TOML key " +
				"theme = \"dark\"",
		)
	}
}
