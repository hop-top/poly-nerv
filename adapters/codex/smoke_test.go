//go:build !short

package codex

import (
	"encoding/json"
	"testing"

	adapter "hop.top/nerv/adapters/internal"
)

// Verify Adapter satisfies HookAdapter at compile time.
var _ adapter.HookAdapter = (*Adapter)(nil)

func TestAdapter_CLI(t *testing.T) {
	a := New()
	if a.CLI() != "codex-cli" {
		t.Errorf("CLI() = %q, want %q", a.CLI(), "codex-cli")
	}
}

func TestAdapter_ConfigPaths(t *testing.T) {
	a := New()
	paths := a.ConfigPaths()
	if len(paths) != 2 {
		t.Fatalf("ConfigPaths() returned %d paths, want 2", len(paths))
	}
	if paths[0] != ".codex/hooks.json" {
		t.Errorf("ConfigPaths()[0] = %q, want .codex/hooks.json", paths[0])
	}
}

func TestAdapter_Capabilities(t *testing.T) {
	a := New()
	caps := a.Capabilities()
	if !caps.PreToolUse {
		t.Error("PreToolUse should be true")
	}
	if !caps.PostToolUse {
		t.Error("PostToolUse should be true")
	}
	if !caps.PreSession {
		t.Error("PreSession should be true")
	}
	if caps.Streaming {
		t.Error("Streaming should be false")
	}
}

func TestAdapter_MergeConfig_Empty(t *testing.T) {
	a := New()
	merged, err := a.MergeConfig(nil)
	if err != nil {
		t.Fatalf("MergeConfig(nil) error: %v", err)
	}

	var cfg codexHooksConfig
	if err := json.Unmarshal(merged, &cfg); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}

	// Should have entries for all 5 events.
	expected := []string{
		"SessionStart", "UserPromptSubmit", "Stop",
		"PreToolUse", "PostToolUse",
	}
	for _, event := range expected {
		entries, ok := cfg.Hooks[event]
		if !ok {
			t.Errorf("missing hook entry for %q", event)
			continue
		}
		if len(entries) == 0 {
			t.Errorf("empty hook entries for %q", event)
		}
	}
}

func TestAdapter_MergeConfig_Existing(t *testing.T) {
	a := New()
	existing := []byte(`{
		"hooks": {
			"SessionStart": [
				{
					"hooks": [
						{"type": "command", "command": "my-custom-hook"}
					]
				}
			]
		}
	}`)

	merged, err := a.MergeConfig(existing)
	if err != nil {
		t.Fatalf("MergeConfig(existing) error: %v", err)
	}

	var cfg codexHooksConfig
	if err := json.Unmarshal(merged, &cfg); err != nil {
		t.Fatalf("unmarshal merged: %v", err)
	}

	// SessionStart should have the custom hook + nerv hook in same entry.
	entries := cfg.Hooks["SessionStart"]
	if len(entries) != 1 {
		t.Fatalf(
			"SessionStart entries = %d, want 1 (merged)", len(entries),
		)
	}
	if len(entries[0].Hooks) != 2 {
		t.Errorf(
			"SessionStart hooks = %d, want 2 (custom + nerv)",
			len(entries[0].Hooks),
		)
	}
}

func TestAdapter_MergeConfig_Idempotent(t *testing.T) {
	a := New()
	first, err := a.MergeConfig(nil)
	if err != nil {
		t.Fatalf("first merge error: %v", err)
	}

	second, err := a.MergeConfig(first)
	if err != nil {
		t.Fatalf("second merge error: %v", err)
	}

	if string(first) != string(second) {
		t.Error("MergeConfig is not idempotent")
	}
}

func TestAdapter_Translate_ValidJSON(t *testing.T) {
	a := New()
	input := []byte(`{"event":"SessionStart","payload":{}}`)
	out, err := a.Translate(input)
	if err != nil {
		t.Fatalf("Translate error: %v", err)
	}
	if string(out) != string(input) {
		t.Error("Translate should pass through valid JSON unchanged")
	}
}

func TestAdapter_Translate_InvalidJSON(t *testing.T) {
	a := New()
	_, err := a.Translate([]byte(`not json`))
	if err == nil {
		t.Error("Translate should error on invalid JSON")
	}
}

func TestAdapter_ShellMatcher(t *testing.T) {
	a := New()
	m := a.ShellMatcher()
	if m == nil {
		t.Fatal("ShellMatcher() returned nil")
	}
	got := m.Match("cd /tmp")
	if len(got) == 0 {
		t.Error("ShellMatcher should match 'cd /tmp'")
	}
}
