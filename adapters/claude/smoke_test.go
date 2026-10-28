//go:build !short

package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestSmoke_RoundTrip exercises the full install-translate-uninstall
// cycle using a temp directory as HOME.
func TestSmoke_RoundTrip(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o750); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}

	// Step 1: Start with empty settings.
	configPath := filepath.Join(claudeDir, "settings.json")
	initial := map[string]any{
		"theme":       "dark",
		"permissions": map[string]any{"allow": []string{"Bash"}},
	}
	initialJSON, err := json.MarshalIndent(initial, "", "  ")
	if err != nil {
		t.Fatalf("marshal initial config: %v", err)
	}
	if err := os.WriteFile(configPath, initialJSON, 0o640); err != nil {
		t.Fatalf("write initial config: %v", err)
	}

	// Step 2: Merge nerv hooks into settings.
	adapter := NewAdapter(home)
	existing, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	merged, err := adapter.MergeConfig(existing)
	if err != nil {
		t.Fatalf("MergeConfig() error: %v", err)
	}
	if err := os.WriteFile(configPath, merged, 0o640); err != nil {
		t.Fatalf("write merged config: %v", err)
	}

	// Step 3: Verify settings.json has hooks + preserves originals.
	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read merged config: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(written, &settings); err != nil {
		t.Fatalf("unmarshal merged config: %v", err)
	}
	if settings["theme"] != "dark" {
		t.Errorf("theme lost after merge: %v", settings["theme"])
	}
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatal("hooks not present after merge")
	}
	if len(hooks) != 25 {
		t.Errorf("hooks count = %d, want 25", len(hooks))
	}

	// Step 4: Translate a test envelope (Claude -> nerv).
	tr := &Translator{}
	claudeEvent := map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "smoke-sess",
		"cwd":             home,
		"tool_name":       "Read",
		"tool_input": map[string]any{
			"file_path": "/tmp/test.txt",
		},
	}
	env, err := tr.ToEnvelope(claudeEvent)
	if err != nil {
		t.Fatalf("ToEnvelope() error: %v", err)
	}
	if env.Event != "PreToolUse" {
		t.Errorf("envelope event = %q, want PreToolUse", env.Event)
	}
	if env.CLI.Name != "claude" {
		t.Errorf("envelope CLI = %q, want claude", env.CLI.Name)
	}

	// Step 5: Convert nerv decision back to Claude format.
	decision := Decision{
		Action: "block",
		Reason: "file access denied by policy",
	}
	out, exitCode, err := tr.FromDecision(decision, "PreToolUse")
	if err != nil {
		t.Fatalf("FromDecision() error: %v", err)
	}
	if exitCode != 2 {
		t.Errorf("exit code = %d, want 2", exitCode)
	}
	var decResult map[string]any
	if err := json.Unmarshal(out, &decResult); err != nil {
		t.Fatalf("unmarshal decision: %v", err)
	}
	hso, ok2 := decResult["hookSpecificOutput"].(map[string]any)
	if !ok2 {
		t.Fatal("missing hookSpecificOutput in decision")
	}
	if hso["permissionDecision"] != "deny" {
		t.Errorf(
			"permissionDecision = %v, want deny",
			hso["permissionDecision"],
		)
	}

	// Step 6: Restore original settings (simulate uninstall).
	if err := os.WriteFile(
		configPath, initialJSON, 0o640,
	); err != nil {
		t.Fatalf("restore config: %v", err)
	}
	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read restored config: %v", err)
	}
	var restoredSettings map[string]any
	if err := json.Unmarshal(restored, &restoredSettings); err != nil {
		t.Fatalf("unmarshal restored config: %v", err)
	}
	if _, hasHooks := restoredSettings["hooks"]; hasHooks {
		t.Error("hooks still present after restore")
	}
	if restoredSettings["theme"] != "dark" {
		t.Error("theme not preserved after restore")
	}
}
