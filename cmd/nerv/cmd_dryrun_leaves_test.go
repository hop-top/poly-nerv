package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// runInProjectEnv is runInProject with extra environment entries
// appended after the XDG sandbox overrides (so callers can redirect
// HOME, PATH, or provider env into the sandbox too).
func runInProjectEnv(t *testing.T, bin, project string, extraEnv []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = project
	cmd.Env = append(os.Environ(),
		"XDG_DATA_HOME="+filepath.Join(project, "xdg-data"),
		"XDG_STATE_HOME="+filepath.Join(project, "xdg-state"),
		"XDG_CONFIG_HOME="+filepath.Join(project, "xdg-config"),
	)
	cmd.Env = append(cmd.Env, extraEnv...)
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

// seedToggleProject writes a .nerv/ project with one enabled hook,
// one disabled hook, and a disabled.yaml carrying one event and one
// binding — enough state for every disable/enable sub-leaf to have
// something it WOULD change.
func seedToggleProject(t *testing.T, project string) (hooksYAML, disabledYAML string) {
	t.Helper()
	nervDir := filepath.Join(project, ".nerv")
	if err := os.MkdirAll(nervDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hooksYAML = filepath.Join(nervDir, "hooks.yaml")
	disabledYAML = filepath.Join(nervDir, "disabled.yaml")
	if err := os.WriteFile(hooksYAML, []byte(`version: "1.0-draft"
# keep this comment
hooks:
  - name: live-hook
    on: PreToolUse
    run: ./hooks/live.sh
  - name: dark-hook
    on: PostToolUse
    run: ./hooks/dark.sh
    disabled: true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(disabledYAML, []byte(`version: 1.0-draft
events:
    - SessionStart
bindings:
    - hook: live-hook
      event: PostToolUse
`), 0o644); err != nil {
		t.Fatal(err)
	}
	return hooksYAML, disabledYAML
}

// TestDisableEnableDryRun_AppliesNothingAndEmitsPlan locks in
// factor-6 previewability for all six disable/enable sub-leaves:
// --dry-run must leave hooks.yaml and disabled.yaml byte-identical,
// must emit a structured plan on stdout in JSON mode, and must exit 0.
func TestDisableEnableDryRun_AppliesNothingAndEmitsPlan(t *testing.T) {
	bin := buildNerv(t)

	cases := []struct {
		name string
		args []string
	}{
		{"disable-hooks", []string{"disable", "hooks", "live-hook"}},
		{"disable-events", []string{"disable", "events", "PreToolUse"}},
		{"disable-bindings", []string{"disable", "bindings", "live-hook:PreCompact"}},
		{"enable-hooks", []string{"enable", "hooks", "dark-hook"}},
		{"enable-events", []string{"enable", "events", "SessionStart"}},
		{"enable-bindings", []string{"enable", "bindings", "live-hook:PostToolUse"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			hooksYAML, disabledYAML := seedToggleProject(t, project)
			beforeHooks, err := os.ReadFile(hooksYAML)
			if err != nil {
				t.Fatal(err)
			}
			beforeDisabled, err := os.ReadFile(disabledYAML)
			if err != nil {
				t.Fatal(err)
			}

			args := append(append([]string{}, tc.args...),
				"--no-confirm", "--dry-run", "--format", "json")
			stdout, stderr, code := runInProject(t, bin, project, args...)
			if code != 0 {
				t.Fatalf("dry-run exited %d, want 0\nstdout:\n%s\nstderr:\n%s",
					code, stdout, stderr)
			}

			// Nothing may be applied: both files stay byte-identical.
			afterHooks, err := os.ReadFile(hooksYAML)
			if err != nil {
				t.Fatal(err)
			}
			afterDisabled, err := os.ReadFile(disabledYAML)
			if err != nil {
				t.Fatal(err)
			}
			if string(beforeHooks) != string(afterHooks) {
				t.Errorf("hooks.yaml mutated under --dry-run\nbefore:\n%s\nafter:\n%s",
					beforeHooks, afterHooks)
			}
			if string(beforeDisabled) != string(afterDisabled) {
				t.Errorf("disabled.yaml mutated under --dry-run\nbefore:\n%s\nafter:\n%s",
					beforeDisabled, afterDisabled)
			}

			// Preview must be a structured plan with at least one effect.
			var plan planDoc
			if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
				t.Fatalf("stdout is not a JSON plan: %v\nstdout:\n%s", err, stdout)
			}
			if len(plan.Effects) == 0 {
				t.Errorf("plan has no effects\nstdout:\n%s", stdout)
			}
		})
	}
}

// TestDisableDryRun_NeedsNoConfirmation locks in that the preview
// path is non-interactive: --dry-run must succeed without
// --no-confirm even when stdin is not a TTY, because it applies
// nothing there is nothing to confirm.
func TestDisableDryRun_NeedsNoConfirmation(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()
	seedToggleProject(t, project)

	stdout, stderr, code := runInProject(t, bin, project,
		"disable", "hooks", "live-hook", "--dry-run", "--format", "json")
	if code != 0 {
		t.Fatalf("dry-run without --no-confirm exited %d, want 0\nstdout:\n%s\nstderr:\n%s",
			code, stdout, stderr)
	}
}

// TestDaemonReloadDryRun_SendsNoSignalAndEmitsPlan locks in that
// `daemon reload --dry-run` does not deliver SIGHUP: the target
// process must stay alive, and stdout must carry a structured plan
// describing the signal that WOULD be sent.
func TestDaemonReloadDryRun_SendsNoSignalAndEmitsPlan(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()

	// Fake daemon: a sleep child whose default SIGHUP disposition is
	// terminate — if reload signals it under --dry-run, it dies.
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatalf("starting fake daemon: %v", err)
	}
	defer func() {
		_ = child.Process.Kill()
		_, _ = child.Process.Wait()
	}()

	stateDir := filepath.Join(project, "xdg-state", "nerv")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	pidfile := filepath.Join(stateDir, "daemon.pid")
	if err := os.WriteFile(pidfile,
		[]byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runInProject(t, bin, project,
		"daemon", "reload", "--dry-run", "--format", "json")
	if code != 0 {
		t.Fatalf("reload --dry-run exited %d, want 0\nstdout:\n%s\nstderr:\n%s",
			code, stdout, stderr)
	}

	// Give any (buggy) signal time to land, then check liveness.
	time.Sleep(200 * time.Millisecond)
	if err := child.Process.Signal(syscall.Signal(0)); err != nil {
		t.Errorf("fake daemon died: SIGHUP was delivered under --dry-run (%v)", err)
	}

	var plan planDoc
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("stdout is not a JSON plan: %v\nstdout:\n%s", err, stdout)
	}
	if len(plan.Effects) == 0 {
		t.Errorf("plan has no effects\nstdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "reload signal sent") {
		t.Errorf("dry-run stdout claims the signal was sent:\n%s", stdout)
	}
}

// TestInstallAddDryRun_AppliesNothingAndEmitsPlan locks in that
// `install add <cli> --dry-run` writes nothing at all — not even the
// nerv state directory — and emits a structured plan describing the
// config merge it WOULD perform.
func TestInstallAddDryRun_AppliesNothingAndEmitsPlan(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()

	// Fake `claude` on PATH so adapter detection succeeds
	// deterministically without the real CLI.
	fakeBin := filepath.Join(project, "fakebin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakeBin, "claude"),
		[]byte("#!/bin/sh\necho 1.2.3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	fakeHome := filepath.Join(project, "fakehome")
	if err := os.MkdirAll(fakeHome, 0o755); err != nil {
		t.Fatal(err)
	}

	env := []string{
		"PATH=" + fakeBin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME=" + fakeHome,
	}
	stdout, stderr, code := runInProjectEnv(t, bin, project, env,
		"install", "add", "claude", "--dry-run", "--format", "json")
	if code != 0 {
		t.Fatalf("install add --dry-run exited %d, want 0\nstdout:\n%s\nstderr:\n%s",
			code, stdout, stderr)
	}

	// Nothing may be applied — no state dir, no CLI config, no journal.
	if _, err := os.Stat(filepath.Join(project, "xdg-state")); !os.IsNotExist(err) {
		t.Errorf("nerv state dir created under --dry-run (stat err: %v)", err)
	}
	if _, err := os.Stat(filepath.Join(fakeHome, ".claude")); !os.IsNotExist(err) {
		t.Errorf("CLI config written under --dry-run (stat err: %v)", err)
	}

	var plan planDoc
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("stdout is not a JSON plan: %v\nstdout:\n%s", err, stdout)
	}
	if len(plan.Effects) == 0 {
		t.Errorf("plan has no effects\nstdout:\n%s", stdout)
	}
	if strings.Contains(stdout, "dry-run OK") {
		t.Errorf("dry-run stdout is the legacy text notice, not a plan:\n%s", stdout)
	}
}

// TestCreateDryRun_NoProviderCallAndEmitsPlan locks in that
// `create --dry-run` previews WITHOUT resolving or invoking an LLM
// provider: in a sandbox with no provider configured it must still
// exit 0 with a structured plan, and must write nothing.
func TestCreateDryRun_NoProviderCallAndEmitsPlan(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()
	fakeHome := filepath.Join(project, "fakehome")
	if err := os.MkdirAll(fakeHome, 0o755); err != nil {
		t.Fatal(err)
	}

	env := []string{
		"HOME=" + fakeHome,
		"LLM_PROVIDER=",
	}
	stdout, stderr, code := runInProjectEnv(t, bin, project, env,
		"create", "block dangerous removals", "--name", "blocker",
		"--dry-run", "--format", "json")
	if code != 0 {
		t.Fatalf("create --dry-run exited %d, want 0 (must not need a provider)\nstdout:\n%s\nstderr:\n%s",
			code, stdout, stderr)
	}

	if _, err := os.Stat(filepath.Join(project, ".nerv")); !os.IsNotExist(err) {
		t.Errorf(".nerv project dir exists after create --dry-run (stat err: %v)", err)
	}
	if _, err := os.Stat(filepath.Join(project, "xdg-data")); !os.IsNotExist(err) {
		t.Errorf("handler data dir exists after create --dry-run (stat err: %v)", err)
	}

	var plan planDoc
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("stdout is not a JSON plan: %v\nstdout:\n%s", err, stdout)
	}
	if len(plan.Effects) == 0 {
		t.Errorf("plan has no effects\nstdout:\n%s", stdout)
	}
}

// TestUpgradeDryRun_EmitsPlan locks in previewability for the
// upgrade stub: --dry-run must produce a structured plan (not the
// human notice) and exit 0.
func TestUpgradeDryRun_EmitsPlan(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()

	stdout, stderr, code := runInProject(t, bin, project,
		"upgrade", "--dry-run", "--format", "json")
	if code != 0 {
		t.Fatalf("upgrade --dry-run exited %d, want 0\nstdout:\n%s\nstderr:\n%s",
			code, stdout, stderr)
	}

	var plan planDoc
	if err := json.Unmarshal([]byte(stdout), &plan); err != nil {
		t.Fatalf("stdout is not a JSON plan: %v\nstdout:\n%s", err, stdout)
	}
	if !strings.Contains(plan.Command, "upgrade") {
		t.Errorf("plan command = %q, want it to reference upgrade", plan.Command)
	}
	if strings.Contains(stdout, "Version check not yet available") {
		t.Errorf("dry-run stdout is the human notice, not a plan:\n%s", stdout)
	}
}
