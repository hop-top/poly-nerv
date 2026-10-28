//go:build !windows

package dispatch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// writeScript creates an executable shell script in dir and returns its path.
func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// testEnvelope returns a minimal envelope for tests.
func testEnvelope() *envelope.Envelope {
	return &envelope.Envelope{
		NervVersion: "0.1.0",
		Event:       "pre_tool_use",
		EventID:     "evt-test-001",
		Timestamp:   time.Now(),
		CLI:         envelope.CLIInfo{Name: "test-cli", Version: "1.0.0"},
		Session:     envelope.SessionInfo{ID: "sess-abc", CWD: "/tmp/test"},
		Payload:     json.RawMessage(`{"tool_name":"bash"}`),
	}
}

// testConfig returns a HooksConfig with test-appropriate defaults.
func testConfig() *config.HooksConfig {
	return &config.HooksConfig{
		Defaults: config.HookDefaults{Timeout: 10, OnError: "continue"},
	}
}

func TestRunHooks_EmptyList(t *testing.T) {
	r := NewRunner()
	decisions := r.RunHooks(nil, testEnvelope(), testConfig())
	if len(decisions) != 0 {
		t.Fatalf("expected 0 decisions, got %d", len(decisions))
	}
}

func TestRunHooks_AllowHandler(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "allow.sh", "#!/bin/sh\nexit 0\n")

	r := NewRunner()
	hooks := []config.HookEntry{{Name: "allow-hook", Run: script}}
	decisions := r.RunHooks(hooks, testEnvelope(), testConfig())

	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Action != merger.Allow {
		t.Errorf("expected allow, got %s", decisions[0].Action)
	}
}

func TestRunHooks_BlockHandler(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "block.sh",
		`#!/bin/sh
echo '{"action":"block","message":"not allowed"}'
exit 2
`)

	r := NewRunner()
	hooks := []config.HookEntry{{Name: "block-hook", Run: script}}
	decisions := r.RunHooks(hooks, testEnvelope(), testConfig())

	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Action != merger.Block {
		t.Errorf("expected block, got %s", decisions[0].Action)
	}
	if decisions[0].Message != "not allowed" {
		t.Errorf("expected 'not allowed' message, got %q", decisions[0].Message)
	}
}

func TestRunHooks_WarnHandler(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "warn.sh",
		`#!/bin/sh
echo '{"action":"warn","message":"proceed with caution"}'
exit 1
`)

	r := NewRunner()
	hooks := []config.HookEntry{{Name: "warn-hook", Run: script}}
	decisions := r.RunHooks(hooks, testEnvelope(), testConfig())

	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Action != merger.Warn {
		t.Errorf("expected warn, got %s", decisions[0].Action)
	}
	if decisions[0].Message != "proceed with caution" {
		t.Errorf("expected 'proceed with caution', got %q", decisions[0].Message)
	}
}

func TestRunHooks_ErrorHandler(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "error.sh",
		"#!/bin/sh\necho 'handler crashed' >&2\nexit 4\n")

	r := NewRunner()
	hooks := []config.HookEntry{{Name: "error-hook", Run: script}}
	decisions := r.RunHooks(hooks, testEnvelope(), testConfig())

	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Action != merger.Allow {
		t.Errorf("expected allow (fail-open), got %s", decisions[0].Action)
	}
	if decisions[0].Message == "" {
		t.Error("expected fail-open message, got empty")
	}
}

func TestRunHooks_EmptyStdout(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "empty.sh", "#!/bin/sh\nexit 0\n")

	r := NewRunner()
	hooks := []config.HookEntry{{Name: "empty-hook", Run: script}}
	decisions := r.RunHooks(hooks, testEnvelope(), testConfig())

	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Action != merger.Allow {
		t.Errorf("expected allow, got %s", decisions[0].Action)
	}
}

func TestRunHooks_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "bad.sh",
		"#!/bin/sh\necho 'not json at all'\nexit 0\n")

	r := NewRunner()
	hooks := []config.HookEntry{{Name: "bad-json-hook", Run: script}}
	decisions := r.RunHooks(hooks, testEnvelope(), testConfig())

	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Action != merger.Allow {
		t.Errorf("expected allow (fail-open), got %s", decisions[0].Action)
	}
	if decisions[0].Message == "" {
		t.Error("expected fail-open message, got empty")
	}
}

func TestRunHooks_EnvVars(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "env.out")

	script := writeScript(t, dir, "env.sh", fmt.Sprintf(
		`#!/bin/sh
echo "EVENT=$NERV_EVENT" >> %s
echo "CLI=$NERV_CLI" >> %s
echo "SESSION=$NERV_SESSION_ID" >> %s
echo "CWD=$NERV_CWD" >> %s
echo "HOOK=$NERV_HOOK_NAME" >> %s
exit 0
`, outFile, outFile, outFile, outFile, outFile))

	env := testEnvelope()
	r := NewRunner()
	hooks := []config.HookEntry{{Name: "env-check", Run: script}}
	r.RunHooks(hooks, env, testConfig())

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read env output: %v", err)
	}
	output := string(data)

	checks := map[string]string{
		"EVENT=":   env.Event,
		"CLI=":     env.CLI.Name,
		"SESSION=": env.Session.ID,
		"CWD=":     env.Session.CWD,
		"HOOK=":    "env-check",
	}
	for prefix, want := range checks {
		expected := prefix + want
		found := false
		for _, line := range splitLines(output) {
			if line == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in output:\n%s", expected, output)
		}
	}
}

func TestRunHooks_Timeout(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "slow.sh", "#!/bin/sh\nsleep 30\nexit 0\n")

	timeout := 1
	r := NewRunner()
	hooks := []config.HookEntry{{
		Name:    "slow-hook",
		Run:     script,
		Timeout: &timeout,
	}}

	start := time.Now()
	decisions := r.RunHooks(hooks, testEnvelope(), testConfig())
	elapsed := time.Since(start)

	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].Action != merger.Allow {
		t.Errorf("expected allow (fail-open on timeout), got %s", decisions[0].Action)
	}
	if elapsed > 5*time.Second {
		t.Errorf("timeout took too long: %v (expected ~1s)", elapsed)
	}
}

// splitLines splits a string into non-empty lines.
func splitLines(s string) []string {
	var lines []string
	for _, l := range splitString(s, '\n') {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// splitString splits s by sep rune into parts.
func splitString(s string, sep rune) []string {
	var parts []string
	current := ""
	for _, r := range s {
		if r == sep {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(r)
		}
	}
	parts = append(parts, current)
	return parts
}
