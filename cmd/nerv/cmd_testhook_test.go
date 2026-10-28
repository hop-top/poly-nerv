//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/dispatch"
)

// TestTestCmd_FlagsReachRunE locks in the fix for the viper-binding
// defect: testCmd registers --event/--all as local flags but they
// were previously read via the package-global viper instance, which
// kit never binds to. The result was that every invocation of
// `nerv test --event <e>` failed with "--event or --all required"
// regardless of what flags were passed.
//
// Regression: read flags off cmd.Flags() instead of viper.
func TestTestCmd_FlagsReachRunE(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate from any user hooks.yaml
	chdirTemp(t)                  // isolate from project hooks.yaml

	cmd := testCmd()
	if err := cmd.Flags().Set("event", "PreToolUse"); err != nil {
		t.Fatalf("set --event: %v", err)
	}

	// With the fix in place, runTest reads --event off cmd.Flags()
	// and proceeds past the "--event or --all required" guard. With
	// the defect, this returns that exact error.
	err := runTest(cmd, nil)
	if err != nil && strings.Contains(err.Error(), "--event or --all required") {
		t.Fatalf("flag --event did not reach RunE: %v", err)
	}
	if err != nil {
		t.Fatalf("runTest returned unexpected error: %v", err)
	}
}

func TestTestCmd_AllFlagReachesRunE(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	chdirTemp(t)

	cmd := testCmd()
	if err := cmd.Flags().Set("all", "true"); err != nil {
		t.Fatalf("set --all: %v", err)
	}

	err := runTest(cmd, nil)
	if err != nil && strings.Contains(err.Error(), "--event or --all required") {
		t.Fatalf("flag --all did not reach RunE: %v", err)
	}
	if err != nil {
		t.Fatalf("runTest returned unexpected error: %v", err)
	}
}

func TestTestCmd_NoFlagStillErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	chdirTemp(t)

	cmd := testCmd()

	err := runTest(cmd, nil)
	if err == nil {
		t.Fatal("expected error when neither --event nor --all is set")
	}
	if !strings.Contains(err.Error(), "--event or --all required") {
		t.Fatalf("expected '--event or --all required', got: %v", err)
	}
}

// chdirTemp switches the test process into an empty temp directory so
// config.Load() finds no project-scoped hooks.yaml.
func chdirTemp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
}

// TestBuildSyntheticEnvelope_PayloadShape pins the synthetic payload to
// the canonical PreToolUse shape (spec/events.yaml): the command lives
// under tool_input, as the adapters produce it, not at the top level.
func TestBuildSyntheticEnvelope_PayloadShape(t *testing.T) {
	env := buildSyntheticEnvelope("PreToolUse", "Bash", "rm -rf /")

	var payload struct {
		ToolName  string         `json:"tool_name"`
		ToolInput map[string]any `json:"tool_input"`
		Command   any            `json:"command"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ToolName != "Bash" {
		t.Errorf("tool_name = %q, want Bash", payload.ToolName)
	}
	if got := payload.ToolInput["command"]; got != "rm -rf /" {
		t.Errorf("tool_input.command = %v, want %q", got, "rm -rf /")
	}
	if payload.Command != nil {
		t.Errorf("top-level command = %v, want absent", payload.Command)
	}
}

// TestTestCmd_QuickStartHandlerBlocks runs the README quick-start
// handler through the same path as `nerv test --command`.
func TestTestCmd_QuickStartHandlerBlocks(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not in PATH")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	chdirTemp(t)

	script := `#!/bin/sh
CMD=$(jq -r '.payload.tool_input.command // empty')
if echo "$CMD" | grep -qE 'rm[[:space:]]+-rf[[:space:]]+/'; then
  echo '{"action":"block","message":"rm -rf / blocked by policy"}'
  exit 2
fi
exit 0
`
	hooksYAML := `version: "1.0-draft"
hooks:
  - name: block-rm-rf
    on: PreToolUse
    match: "Bash|shell|local_shell"
    run: sh ./block-rm-rf.sh
`
	if err := os.WriteFile("block-rm-rf.sh", []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".nerv", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".nerv", "hooks.yaml"), []byte(hooksYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	env := buildSyntheticEnvelope("PreToolUse", "Bash", "rm -rf /")
	results := testSingleEvent(
		config.Compile(cfg), dispatch.NewMatcher(), dispatch.NewRunner(), env,
	)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(results), results)
	}
	if results[0].Decision != "block" {
		t.Fatalf("decision = %q, want block (%+v)", results[0].Decision, results[0])
	}
}
