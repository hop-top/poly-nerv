package claude

import (
	"encoding/json"
	"testing"

	"hop.top/nerv/adapters/internal"
)

func TestMergeConfig_Fresh(t *testing.T) {
	out, err := mergeHooksIntoSettings(nil)
	if err != nil {
		t.Fatalf("mergeHooksIntoSettings() error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(out, &settings); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("missing hooks key")
	}

	// All 25 managed events should be present; WorktreeCreate must NOT
	// be — its stdout is a worktree-path delegation contract nerv
	// cannot fulfill, and registering nervd there breaks isolated
	// worktree creation entirely.
	if len(hooks) != 25 {
		t.Errorf("hooks count = %d, want 25", len(hooks))
	}
	if _, ok := hooks["WorktreeCreate"]; ok {
		t.Error("WorktreeCreate registered; nerv must not manage it")
	}
	if _, ok := hooks["WorktreeRemove"]; !ok {
		t.Error("WorktreeRemove missing; observational event should be managed")
	}

	// Spot-check PreToolUse structure.
	entries, ok := hooks["PreToolUse"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatal("missing PreToolUse entries")
	}
	entry := entries[0].(map[string]any)
	hooksList := entry["hooks"].([]any)
	hook := hooksList[0].(map[string]any)
	if hook["command"] != "nervd" {
		t.Errorf(
			"hook command = %v, want %q",
			hook["command"], "nervd",
		)
	}
	if hook["type"] != "command" {
		t.Errorf(
			"hook type = %v, want %q",
			hook["type"], "command",
		)
	}
}

func TestMergeConfig_Existing(t *testing.T) {
	existing := []byte(`{
  "permissions": {"allow": ["Bash"]},
  "theme": "dark"
}`)

	out, err := mergeHooksIntoSettings(existing)
	if err != nil {
		t.Fatalf("mergeHooksIntoSettings() error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(out, &settings); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	// Non-hook settings preserved.
	if settings["theme"] != "dark" {
		t.Errorf("theme = %v, want dark", settings["theme"])
	}
	perms, ok := settings["permissions"].(map[string]any)
	if !ok {
		t.Fatal("missing permissions")
	}
	if _, ok := perms["allow"]; !ok {
		t.Error("missing permissions.allow")
	}

	// Hooks added.
	hooks := settings["hooks"].(map[string]any)
	if len(hooks) != 25 {
		t.Errorf("hooks count = %d, want 25", len(hooks))
	}
}

// TestMergeConfig_StripsStaleWorktreeCreate covers the upgrade path:
// settings written by older nerv versions carry a nervd WorktreeCreate
// entry that breaks isolated worktree creation. Re-merging must drop
// it while leaving user-owned WorktreeCreate hooks untouched.
func TestMergeConfig_StripsStaleWorktreeCreate(t *testing.T) {
	existing := []byte(`{
  "hooks": {
    "WorktreeCreate": [
      {"matcher": "", "hooks": [{"type": "command", "command": "nervd"}]},
      {"matcher": "", "hooks": [{"type": "command", "command": "my-worktree-maker"}]}
    ]
  }
}`)

	out, err := mergeHooksIntoSettings(existing)
	if err != nil {
		t.Fatalf("mergeHooksIntoSettings() error: %v", err)
	}

	var settings map[string]any
	if err := json.Unmarshal(out, &settings); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	hooks := settings["hooks"].(map[string]any)

	entries, ok := hooks["WorktreeCreate"].([]any)
	if !ok {
		t.Fatal("user WorktreeCreate entry dropped entirely")
	}
	if len(entries) != 1 {
		t.Fatalf("WorktreeCreate entries = %d, want 1 (user hook only)", len(entries))
	}
	if isNervEntry(entries[0]) {
		t.Error("stale nervd WorktreeCreate entry survived merge")
	}
}

func TestMergeConfig_Overwrite(t *testing.T) {
	// First merge: install nerv hooks.
	first, err := mergeHooksIntoSettings(nil)
	if err != nil {
		t.Fatalf("first merge error: %v", err)
	}

	// Add a non-nerv hook to the settings.
	var settings map[string]any
	if err := json.Unmarshal(first, &settings); err != nil {
		t.Fatalf("unmarshal first: %v", err)
	}
	hooks := settings["hooks"].(map[string]any)

	// Add a user hook to PreToolUse alongside nerv.
	userHook := map[string]any{
		"matcher": "Write",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": "my-linter",
			},
		},
	}
	existing := hooks["PreToolUse"].([]any)
	hooks["PreToolUse"] = append(existing, userHook)
	settings["hooks"] = hooks
	modified, _ := json.Marshal(settings)

	// Second merge: should replace nerv hooks, keep user hook.
	out, err := mergeHooksIntoSettings(modified)
	if err != nil {
		t.Fatalf("second merge error: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	resultHooks := result["hooks"].(map[string]any)
	ptEntries := resultHooks["PreToolUse"].([]any)

	// Should have 2 entries: user hook + nerv hook.
	if len(ptEntries) != 2 {
		t.Errorf(
			"PreToolUse entries = %d, want 2",
			len(ptEntries),
		)
	}

	// Verify one is the user hook.
	foundUser := false
	for _, e := range ptEntries {
		em := e.(map[string]any)
		if em["matcher"] == "Write" {
			foundUser = true
		}
	}
	if !foundUser {
		t.Error("user hook not preserved after overwrite")
	}
}

func TestAdapter_ConfigPaths(t *testing.T) {
	a := NewAdapter("/tmp/fakehome")
	paths := a.ConfigPaths()

	if len(paths) != 2 {
		t.Fatalf("ConfigPaths() len = %d, want 2", len(paths))
	}
	if paths[0] != "/tmp/fakehome/.claude/settings.json" {
		t.Errorf(
			"paths[0] = %q, want %q",
			paths[0],
			"/tmp/fakehome/.claude/settings.json",
		)
	}
	if paths[1] != ".claude/settings.json" {
		t.Errorf(
			"paths[1] = %q, want %q",
			paths[1],
			".claude/settings.json",
		)
	}
}

func TestAdapter_CLI(t *testing.T) {
	a := NewAdapter("")
	if a.CLI() != "claude" {
		t.Errorf("CLI() = %q, want %q", a.CLI(), "claude")
	}
}

func TestAdapter_Capabilities(t *testing.T) {
	a := NewAdapter("")
	caps := a.Capabilities()
	if !caps.PreToolUse {
		t.Error("PreToolUse = false, want true")
	}
	if !caps.PostToolUse {
		t.Error("PostToolUse = false, want true")
	}
	if !caps.PreSession {
		t.Error("PreSession = false, want true")
	}
	if !caps.PostSession {
		t.Error("PostSession = false, want true")
	}
}

func TestAdapter_Interface(t *testing.T) {
	// Compile-time check: Adapter satisfies HookAdapter.
	var _ internal.HookAdapter = (*Adapter)(nil)
}
