package codex

import (
	"bytes"
	"encoding/json"
	"testing"
)

// MergeConfig must round-trip every key it does not own: top-level
// keys beside "hooks" and unknown fields on entries and hook commands.
func TestAdapter_MergeConfig_PreservesUnknownFields(t *testing.T) {
	existing := []byte(`{
		"existing_key": "preserved",
		"version": 9007199254740993,
		"hooks": {
			"PreToolUse": [
				{
					"matcher": "shell",
					"description": "user entry",
					"hooks": [
						{"type": "command", "command": "audit", "timeout": 30}
					]
				}
			]
		}
	}`)

	merged, err := New().MergeConfig(existing)
	if err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}

	dec := json.NewDecoder(bytes.NewReader(merged))
	dec.UseNumber()
	var got map[string]any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("merged output not JSON: %v\n%s", err, merged)
	}

	if got["existing_key"] != "preserved" {
		t.Errorf("existing_key = %v, want preserved", got["existing_key"])
	}
	if v, _ := got["version"].(json.Number); v.String() != "9007199254740993" {
		t.Errorf("version = %v, want 9007199254740993 (exact)", got["version"])
	}

	hooks := got["hooks"].(map[string]any)
	entries := hooks["PreToolUse"].([]any)
	if len(entries) != 1 {
		t.Fatalf("PreToolUse entries = %d, want 1 (merged)", len(entries))
	}
	entry := entries[0].(map[string]any)
	if entry["description"] != "user entry" {
		t.Errorf("entry description = %v, want user entry", entry["description"])
	}
	cmds := entry["hooks"].([]any)
	if len(cmds) != 2 {
		t.Fatalf("PreToolUse hooks = %d, want 2 (user + nerv)", len(cmds))
	}
	user := cmds[0].(map[string]any)
	if n, _ := user["timeout"].(json.Number); n.String() != "30" {
		t.Errorf("user hook timeout = %v, want 30", user["timeout"])
	}
}

// A non-object "hooks" value is a schema error, not something to
// silently overwrite.
func TestAdapter_MergeConfig_RejectsMalformedHooks(t *testing.T) {
	for _, in := range []string{
		`{"hooks": []}`,
		`{"hooks": {"SessionStart": {}}}`,
		`{"hooks": {"SessionStart": ["x"]}}`,
	} {
		if _, err := New().MergeConfig([]byte(in)); err == nil {
			t.Errorf("MergeConfig(%s): want error, got nil", in)
		}
	}
}

func TestAdapter_MergeConfig_NullInput(t *testing.T) {
	merged, err := New().MergeConfig([]byte("null"))
	if err != nil {
		t.Fatalf("MergeConfig(null): %v", err)
	}
	var cfg codexHooksConfig
	if err := json.Unmarshal(merged, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cfg.Hooks["SessionStart"]) != 1 {
		t.Errorf("SessionStart entries = %d, want 1", len(cfg.Hooks["SessionStart"]))
	}
}
