package aider

import "testing"

// The config nerv installs wires only lint-cmd/test-cmd, which fire
// around edits. No hook fires at process start or exit, so session
// capabilities must not be claimed (capabilities.yaml lists them under
// synthesized, not native).
func TestCapabilities_NoSessionHook(t *testing.T) {
	caps := (&Adapter{}).Capabilities()
	if caps.PreSession {
		t.Error("PreSession = true; no aider hook fires at session start")
	}
	if caps.PostSession {
		t.Error("PostSession = true; no aider hook fires at session end")
	}
	// lint-cmd/test-cmd run after the edit is on disk
	// (aider/coders/base_coder.py), so nothing precedes tool use.
	if caps.PreToolUse {
		t.Error("PreToolUse = true; lint-cmd/test-cmd run after the edit")
	}
	if !caps.PostToolUse {
		t.Error("PostToolUse must stay true (test-cmd)")
	}
}
