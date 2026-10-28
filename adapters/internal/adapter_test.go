package internal

import (
	"testing"
)

// mockAdapter implements HookAdapter for testing.
type mockAdapter struct {
	cli          CLIName
	detectResult DetectResult
	detectErr    error
	caps         CapabilityMap
	configPaths  []string
	mergeResult  []byte
	mergeErr     error
	translateOut []byte
	translateErr error
}

func (m *mockAdapter) CLI() CLIName                  { return m.cli }
func (m *mockAdapter) Detect() (DetectResult, error) { return m.detectResult, m.detectErr }
func (m *mockAdapter) Capabilities() CapabilityMap   { return m.caps }
func (m *mockAdapter) ConfigPaths() []string         { return m.configPaths }
func (m *mockAdapter) MergeConfig(existing []byte) ([]byte, error) {
	return m.mergeResult, m.mergeErr
}
func (m *mockAdapter) Translate(envelope []byte) ([]byte, error) {
	return m.translateOut, m.translateErr
}

func TestHookAdapterInterface(t *testing.T) {
	// Verify mockAdapter satisfies HookAdapter at compile time.
	var _ HookAdapter = (*mockAdapter)(nil)

	m := &mockAdapter{
		cli: "test-cli",
		detectResult: DetectResult{
			Found: true,
			Info:  CLIInfo{Name: "test-cli", Version: "1.0.0", Path: "/usr/bin/test"},
		},
		caps: CapabilityMap{
			PreToolUse:  true,
			PostToolUse: true,
		},
		configPaths: []string{"/tmp/test.json"},
		mergeResult: []byte(`{"hooks":true}`),
	}

	if m.CLI() != "test-cli" {
		t.Errorf("CLI() = %q, want %q", m.CLI(), "test-cli")
	}

	dr, err := m.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}
	if !dr.Found {
		t.Error("Detect().Found = false, want true")
	}
	if dr.Info.Version != "1.0.0" {
		t.Errorf("Detect().Info.Version = %q, want %q", dr.Info.Version, "1.0.0")
	}

	caps := m.Capabilities()
	if !caps.PreToolUse {
		t.Error("Capabilities().PreToolUse = false, want true")
	}

	paths := m.ConfigPaths()
	if len(paths) != 1 || paths[0] != "/tmp/test.json" {
		t.Errorf("ConfigPaths() = %v, want [/tmp/test.json]", paths)
	}

	merged, err := m.MergeConfig(nil)
	if err != nil {
		t.Fatalf("MergeConfig() error: %v", err)
	}
	if string(merged) != `{"hooks":true}` {
		t.Errorf("MergeConfig() = %q, want %q", merged, `{"hooks":true}`)
	}
}
