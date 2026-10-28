package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// planDoc mirrors the kit cli.Plan JSON shape closely enough to assert
// the preview is structured: a command path plus a non-empty effects
// list describing what WOULD change.
type planDoc struct {
	Command string `json:"command"`
	Effects []struct {
		Kind   string `json:"kind"`
		Target string `json:"target"`
	} `json:"effects"`
}

// runInProject executes the built nerv binary inside an isolated
// project dir with XDG state redirected into the same sandbox, so a
// mutation bug cannot leak outside the test.
func runInProject(t *testing.T, bin, project string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = project
	cmd.Env = append(os.Environ(),
		"XDG_DATA_HOME="+filepath.Join(project, "xdg-data"),
		"XDG_STATE_HOME="+filepath.Join(project, "xdg-state"),
		"XDG_CONFIG_HOME="+filepath.Join(project, "xdg-config"),
	)
	var out, errOut strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running nerv %v: %v", args, err)
	}
	return out.String(), errOut.String(), code
}

func writeHandler(t *testing.T, project string) string {
	t.Helper()
	handler := filepath.Join(project, "handler.sh")
	if err := os.WriteFile(handler, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatalf("writing handler: %v", err)
	}
	return handler
}

// TestAddDryRun_AppliesNothingAndEmitsPlan locks in factor-6
// previewability for `nerv add`: --dry-run must mutate nothing (no
// .nerv/ project dir, no handler under XDG data) and must emit a
// structured plan on stdout in JSON mode, exiting 0.
func TestAddDryRun_AppliesNothingAndEmitsPlan(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()
	writeHandler(t, project)

	stdout, _, code := runInProject(t, bin, project,
		"add", "demo-hook", "--source", "./handler.sh", "--dry-run", "--format", "json")

	if code != 0 {
		t.Fatalf("dry-run exited %d, want 0\nstdout:\n%s", code, stdout)
	}

	// Nothing may be applied.
	if _, err := os.Stat(filepath.Join(project, ".nerv")); !os.IsNotExist(err) {
		t.Errorf(".nerv project dir exists after --dry-run (stat err: %v)", err)
	}
	if _, err := os.Stat(filepath.Join(project, "xdg-data", "nerv")); !os.IsNotExist(err) {
		t.Errorf("handler data dir exists after --dry-run (stat err: %v)", err)
	}

	// Preview must be structured and must not claim application.
	var plan planDoc
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("stdout is not a JSON plan: %v\nstdout:\n%s", err, stdout)
	}
	if len(plan.Effects) == 0 {
		t.Errorf("plan has no effects\nstdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "installed") {
		t.Errorf("dry-run stdout claims installation:\n%s", stdout)
	}

	// State transparency after the preview: status still reports zero hooks.
	statusOut, _, statusCode := runInProject(t, bin, project, "status", "--format", "json")
	if statusCode != 0 {
		t.Fatalf("status exited %d\n%s", statusCode, statusOut)
	}
	if !strings.Contains(statusOut, `"0 defined (version: )"`) {
		t.Errorf("status reports hooks were registered after --dry-run:\n%s", statusOut)
	}
}

// TestRemoveDryRun_AppliesNothing locks in the same guarantee for the
// destructive `nerv remove` leaf: with the confirm gate satisfied,
// --dry-run must not rewrite hooks.yaml / hooks.lock (the buggy path
// re-serialized both even for absent hooks).
func TestRemoveDryRun_AppliesNothing(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()

	stdout, _, code := runInProject(t, bin, project,
		"remove", "some-hook", "--confirm=yes", "--dry-run", "--format", "json")

	if code != 0 {
		t.Fatalf("remove --dry-run exited %d, want 0\nstdout:\n%s", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(project, ".nerv")); !os.IsNotExist(err) {
		t.Errorf(".nerv project dir exists after remove --dry-run (stat err: %v)", err)
	}
	var plan planDoc
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("stdout is not a JSON plan: %v\nstdout:\n%s", err, stdout)
	}
	if len(plan.Effects) == 0 {
		t.Errorf("plan has no effects\nstdout:\n%s", stdout)
	}
}
