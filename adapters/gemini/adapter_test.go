package gemini

import (
	"encoding/json"
	"testing"
)

func TestAdapter_CLI(t *testing.T) {
	a := &Adapter{}
	if a.CLI() != "gemini" {
		t.Errorf("CLI() = %q, want gemini", a.CLI())
	}
}

func TestAdapter_Capabilities(t *testing.T) {
	a := &Adapter{}
	cap := a.Capabilities()
	if !cap.PreToolUse {
		t.Error("expected PreToolUse = true")
	}
	if !cap.PostToolUse {
		t.Error("expected PostToolUse = true")
	}
	if !cap.PreSession {
		t.Error("expected PreSession = true")
	}
	if !cap.PostSession {
		t.Error("expected PostSession = true")
	}
	if cap.Streaming {
		t.Error("expected Streaming = false")
	}
}

func TestAdapter_ConfigPaths(t *testing.T) {
	a := &Adapter{}
	paths := a.ConfigPaths()
	if len(paths) != 2 {
		t.Fatalf("expected 2 config paths, got %d", len(paths))
	}
	// Global path should contain .gemini/settings.json.
	found := false
	for _, p := range paths {
		if len(p) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected non-empty config paths")
	}
}

func TestAdapter_MergeConfig_Empty(t *testing.T) {
	a := &Adapter{}
	out, err := a.MergeConfig(nil)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	hooks, ok := cfg["hooks"].(map[string]any)
	if !ok {
		t.Fatal("missing hooks key")
	}

	// Check all 11 native events are registered.
	expected := []string{
		"SessionStart", "SessionEnd",
		"BeforeTool", "AfterTool",
		"BeforeAgent", "AfterAgent",
		"BeforeModel", "AfterModel",
		"Notification", "PreCompress",
		"BeforeToolSelection",
	}
	for _, ev := range expected {
		if _, ok := hooks[ev]; !ok {
			t.Errorf("missing hook entry for %s", ev)
		}
	}
}

func TestAdapter_MergeConfig_Existing(t *testing.T) {
	a := &Adapter{}
	existing := []byte(`{"theme": "dark", "hooks": {}}`)
	out, err := a.MergeConfig(existing)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	// Existing keys preserved.
	if cfg["theme"] != "dark" {
		t.Error("existing config key lost")
	}

	hooks := cfg["hooks"].(map[string]any)
	if _, ok := hooks["BeforeTool"]; !ok {
		t.Error("BeforeTool hook not added")
	}
}

func TestAdapter_MergeConfig_InvalidJSON(t *testing.T) {
	a := &Adapter{}
	_, err := a.MergeConfig([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
