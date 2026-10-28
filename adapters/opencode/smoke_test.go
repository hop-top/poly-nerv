//go:build !short

package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/nerv/internal/merger"
)

// TestSmoke_EndToEnd verifies the full adapter lifecycle:
// install → translate event → get decision → verify plugin output.
func TestSmoke_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	a := &Adapter{
		ProjectDir: dir,
		NervdPath:  "/usr/local/bin/nervd",
	}

	// Step 1: Install.
	if err := a.Install(); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// Step 2: Verify plugin file.
	pluginPath := filepath.Join(
		dir, ".opencode", "plugins", "nerv-bridge.ts",
	)
	pluginData, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatalf("read plugin: %v", err)
	}
	pluginSrc := string(pluginData)

	if !strings.Contains(pluginSrc, "export default") {
		t.Error("plugin missing export default")
	}
	if !strings.Contains(pluginSrc, `"tool.execute.before"`) {
		t.Error("plugin missing tool.execute.before handler")
	}

	// Step 3: Translate a native event.
	raw := `{
		"type": "tool.execute.before",
		"tool": "bash",
		"input": {"command": "rm -rf /"},
		"session_id": "smoke-test-123"
	}`
	env, err := ToEnvelope([]byte(raw), "fallback")
	if err != nil {
		t.Fatalf("ToEnvelope() error: %v", err)
	}
	if env.Event != "PreToolUse" {
		t.Errorf("Event = %q, want PreToolUse", env.Event)
	}
	if env.Session.ID != "smoke-test-123" {
		t.Errorf("Session.ID = %q, want smoke-test-123", env.Session.ID)
	}

	// Step 4: Produce a block decision and verify wire format.
	d := merger.Decision{
		Action:  merger.Block,
		Message: "use trash, not rm -rf",
	}
	out, err := FromDecision(d)
	if err != nil {
		t.Fatalf("FromDecision() error: %v", err)
	}
	var dj DecisionJSON
	if err := json.Unmarshal(out, &dj); err != nil {
		t.Fatalf("unmarshal decision: %v", err)
	}
	if dj.Action != "block" {
		t.Errorf("decision action = %q, want block", dj.Action)
	}
	if dj.Message != "use trash, not rm -rf" {
		t.Errorf("decision message = %q, want 'use trash, not rm -rf'",
			dj.Message)
	}

	// Step 5: Verify opencode.json has nerv marker.
	configPath := filepath.Join(dir, "opencode.json")
	cfgData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(cfgData, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	nerv, ok := cfg["nerv"].(map[string]any)
	if !ok {
		t.Fatal("nerv key missing from opencode.json")
	}
	if installed, _ := nerv["installed"].(bool); !installed {
		t.Error("nerv.installed != true")
	}

	// Step 6: Verify all native event translations roundtrip.
	nativeEvents := []struct {
		ocEvent  string
		nervName string
	}{
		{OCSessionCreated, "SessionStart"},
		{OCSessionDeleted, "SessionEnd"},
		{OCSessionIdle, "Stop"},
		{OCSessionError, "StopFailure"},
		{OCSessionCompacted, "PostCompact"},
		{OCToolExecBefore, "PreToolUse"},
		{OCToolExecAfter, "PostToolUse"},
		{OCFileEdited, "FileChanged"},
		{OCPermissionAsked, "PermissionRequest"},
		{OCTUIPromptAppend, "UserPromptSubmit"},
		{OCTodoUpdated, "TaskCompleted"},
	}
	for _, tt := range nativeEvents {
		got := TranslateEventName(tt.ocEvent)
		if got != tt.nervName {
			t.Errorf("TranslateEventName(%q) = %q, want %q",
				tt.ocEvent, got, tt.nervName)
		}
	}
}
