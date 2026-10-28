package opencode

import (
	"os"
	"path/filepath"
	"testing"

	"hop.top/nerv/adapters/internal"
)

// Compile-time assertion that Adapter implements HookAdapter.
var _ internal.HookAdapter = (*Adapter)(nil)

func TestAdapter_CLI(t *testing.T) {
	a := &Adapter{}
	if a.CLI() != "opencode" {
		t.Errorf("CLI() = %q, want opencode", a.CLI())
	}
}

func TestAdapter_Capabilities(t *testing.T) {
	a := &Adapter{}
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
	if caps.Streaming {
		t.Error("Streaming = true, want false")
	}
}

func TestAdapter_ConfigPaths(t *testing.T) {
	a := &Adapter{ProjectDir: "/tmp/proj"}
	paths := a.ConfigPaths()
	if len(paths) != 2 {
		t.Fatalf("ConfigPaths() len = %d, want 2", len(paths))
	}
	if paths[0] != "/tmp/proj/opencode.json" {
		t.Errorf("paths[0] = %q, want /tmp/proj/opencode.json", paths[0])
	}
	want := "/tmp/proj/.opencode/plugins/nerv-bridge.ts"
	if paths[1] != want {
		t.Errorf("paths[1] = %q, want %q", paths[1], want)
	}
}

func TestAdapter_MergeConfig(t *testing.T) {
	a := &Adapter{NervdPath: "/usr/local/bin/nervd"}
	out, err := a.MergeConfig(nil)
	if err != nil {
		t.Fatalf("MergeConfig() error: %v", err)
	}
	if len(out) == 0 {
		t.Error("MergeConfig() returned empty output")
	}
}

func TestAdapter_Install(t *testing.T) {
	dir := t.TempDir()
	a := &Adapter{
		ProjectDir: dir,
		NervdPath:  "/usr/local/bin/nervd",
	}

	if err := a.Install(); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// Verify plugin file exists.
	pluginPath := filepath.Join(dir, ".opencode", "plugins", "nerv-bridge.ts")
	if _, err := os.Stat(pluginPath); err != nil {
		t.Errorf("plugin file missing: %v", err)
	}

	// Verify opencode.json exists with nerv marker.
	configPath := filepath.Join(dir, "opencode.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read opencode.json: %v", err)
	}
	if len(data) == 0 {
		t.Error("opencode.json is empty")
	}

	// Re-install should be idempotent.
	if err := a.Install(); err != nil {
		t.Fatalf("second Install() error: %v", err)
	}
}

func TestAdapter_Install_ExistingConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "opencode.json")
	existing := []byte(`{"model":"claude-4","temperature":0.7}` + "\n")
	if err := os.WriteFile(configPath, existing, 0o640); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	a := &Adapter{
		ProjectDir: dir,
		NervdPath:  "/usr/local/bin/nervd",
	}
	if err := a.Install(); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// Verify existing keys preserved.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	s := string(data)
	if !contains(s, `"model"`) {
		t.Error("existing key 'model' lost after install")
	}
	if !contains(s, `"nerv"`) {
		t.Error("nerv marker missing after install")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstr(s, substr)
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
