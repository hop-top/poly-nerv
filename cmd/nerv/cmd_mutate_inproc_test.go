//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runVerb executes args against a throwaway root carrying the kit
// persistent --dry-run flag, in-process, capturing stdout. The kit
// middleware (confirm gate, error envelope) is deliberately absent:
// these tests pin each command's own RunE behavior.
func runVerb(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "nerv", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(
		addCmd(), removeCmd(), disableCmd(), enableCmd(), statusCmd(),
	)
	root.SetArgs(args)
	var err error
	out := captureStdout(t, func() { err = root.Execute() })
	return out, err
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func TestAddRemove_RoundTrip(t *testing.T) {
	project := isolateEnv(t)
	setFormat(t, "json")
	src := writeExe(t, project, "src.sh", "#!/bin/sh\necho ok\n")

	out, err := runVerb(t, "add", "myhook", "--source", src)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	var res hookResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("add output not JSON: %v\n%s", err, out)
	}
	if res.Hook != "myhook" || res.Action != "installed" || res.Source != src {
		t.Errorf("add result = %+v", res)
	}
	if y := readFile(t, filepath.Join(project, ".nerv", "hooks.yaml")); !strings.Contains(y, "myhook") {
		t.Errorf("hooks.yaml missing hook:\n%s", y)
	}
	if l := readFile(t, filepath.Join(project, ".nerv", "hooks.lock")); !strings.Contains(l, "myhook") {
		t.Errorf("hooks.lock missing hook:\n%s", l)
	}
	handler := filepath.Join(os.Getenv("XDG_DATA_HOME"), "nerv", "hooks", "myhook", "handler")
	if _, err := os.Stat(handler); err != nil {
		t.Errorf("handler not installed: %v", err)
	}

	// Remove without purge keeps the handler.
	out, err = runVerb(t, "remove", "myhook")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Action != "removed" || res.Purged {
		t.Errorf("remove result = %+v err=%v", res, err)
	}
	if y := readFile(t, filepath.Join(project, ".nerv", "hooks.yaml")); strings.Contains(y, "myhook") {
		t.Errorf("hook still in hooks.yaml:\n%s", y)
	}
	if _, err := os.Stat(handler); err != nil {
		t.Errorf("handler should survive non-purge remove: %v", err)
	}

	// Re-add then purge deletes the handler dir.
	if _, err := runVerb(t, "add", "myhook", "--source", src); err != nil {
		t.Fatal(err)
	}
	out, err = runVerb(t, "remove", "myhook", "--purge")
	if err != nil {
		t.Fatalf("remove --purge: %v", err)
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || !res.Purged {
		t.Errorf("purge result = %+v err=%v", res, err)
	}
	if _, err := os.Stat(handler); !os.IsNotExist(err) {
		t.Errorf("handler should be purged, stat err = %v", err)
	}
}

func TestAdd_MissingSourceFails(t *testing.T) {
	isolateEnv(t)
	setFormat(t, "json")
	_, err := runVerb(t, "add", "ghost", "--source", "/no/such/handler")
	if err == nil || !strings.Contains(err.Error(), `add hook "ghost"`) {
		t.Errorf("want wrapped add error, got %v", err)
	}
}

func TestAdd_DryRunAppliesNothing(t *testing.T) {
	project := isolateEnv(t)
	setFormat(t, "json")
	out, err := runVerb(t, "add", "dry", "--dry-run")
	if err != nil {
		t.Fatalf("add --dry-run: %v", err)
	}
	var plan planDoc
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("not a plan: %v\n%s", err, out)
	}
	if plan.Command != "nerv add" || len(plan.Effects) != 3 {
		t.Errorf("plan = %+v", plan)
	}
	if !strings.Contains(out, "https://hop.top/nerv/hooks/dry") {
		t.Errorf("default source missing from plan:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(project, ".nerv")); !os.IsNotExist(err) {
		t.Errorf(".nerv created by dry-run: %v", err)
	}
}

func TestRemove_DryRunPurgeAndWarning(t *testing.T) {
	isolateEnv(t)
	setFormat(t, "json")
	out, err := runVerb(t, "remove", "ghost", "--purge", "--dry-run")
	if err != nil {
		t.Fatalf("remove --dry-run: %v", err)
	}
	var plan struct {
		Effects  []struct{ Kind string } `json:"effects"`
		Warnings []string                `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("not a plan: %v\n%s", err, out)
	}
	kinds := ""
	for _, e := range plan.Effects {
		kinds += e.Kind + ","
	}
	if kinds != "update,update,delete," {
		t.Errorf("effect kinds = %q, want update,update,delete,", kinds)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "not in the lockfile") {
		t.Errorf("warnings = %v", plan.Warnings)
	}
}

const mutateHooksYAML = `version: "1"
hooks:
  - name: alpha
    on: PreToolUse
    run: /bin/true
  - name: beta
    on: Stop
    run: /bin/true
`

func setupProject(t *testing.T) string {
	t.Helper()
	project := isolateEnv(t)
	if err := os.MkdirAll(filepath.Join(project, ".nerv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".nerv", "hooks.yaml"), []byte(mutateHooksYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	return project
}

func TestDisableEnableHooks(t *testing.T) {
	project := setupProject(t)
	yamlPath := filepath.Join(project, ".nerv", "hooks.yaml")

	out, err := runVerb(t, "disable", "hooks", "alpha", "nosuch", "--no-confirm")
	if err != nil {
		t.Fatalf("disable hooks: %v", err)
	}
	if !strings.Contains(out, "disabled 1 hooks") || !strings.Contains(out, "unknown (no matching entry): 1") {
		t.Errorf("report:\n%s", out)
	}
	y := readFile(t, yamlPath)
	if !strings.Contains(y, "disabled: true") {
		t.Errorf("alpha not disabled in yaml:\n%s", y)
	}
	if strings.Count(y, "disabled") != 1 {
		t.Errorf("beta must be untouched:\n%s", y)
	}

	// Idempotent.
	out, _ = runVerb(t, "disable", "hooks", "alpha", "--no-confirm")
	if !strings.Contains(out, "no hooks changed (already disabled)") {
		t.Errorf("second disable should be a no-op:\n%s", out)
	}

	out, err = runVerb(t, "enable", "hooks", "alpha", "--no-confirm")
	if err != nil || !strings.Contains(out, "enabled 1 hooks") {
		t.Errorf("enable: err=%v out=\n%s", err, out)
	}
	if y := readFile(t, yamlPath); strings.Contains(y, "disabled") {
		t.Errorf("disabled field not stripped:\n%s", y)
	}
}

func TestDisableEnableEvents(t *testing.T) {
	project := setupProject(t)
	sidecar := filepath.Join(project, ".nerv", "disabled.yaml")

	out, err := runVerb(t, "disable", "events", "PreToolUse", "--no-confirm")
	if err != nil || !strings.Contains(out, "disabled 1 events") {
		t.Fatalf("disable events: err=%v out=\n%s", err, out)
	}
	if !strings.Contains(readFile(t, sidecar), "PreToolUse") {
		t.Errorf("sidecar missing event:\n%s", readFile(t, sidecar))
	}
	out, err = runVerb(t, "enable", "events", "PreToolUse", "--no-confirm")
	if err != nil || !strings.Contains(out, "enabled 1 events") {
		t.Fatalf("enable events: err=%v out=\n%s", err, out)
	}
	if strings.Contains(readFile(t, sidecar), "PreToolUse") {
		t.Errorf("event still in sidecar:\n%s", readFile(t, sidecar))
	}
	out, _ = runVerb(t, "enable", "events", "PreToolUse", "--no-confirm")
	if !strings.Contains(out, "no events changed (already enabled)") {
		t.Errorf("re-enable should be a no-op:\n%s", out)
	}

	// Typo must be rejected before touching the sidecar.
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	_, err = runVerb(t, "disable", "events", "NotAnEvent", "--no-confirm")
	if err == nil || !strings.Contains(err.Error(), "unknown event") {
		t.Errorf("want unknown-event error, got %v", err)
	}
	if _, statErr := os.Stat(sidecar); !os.IsNotExist(statErr) {
		t.Errorf("sidecar written despite invalid event")
	}
}

func TestDisableEnableBindings(t *testing.T) {
	project := setupProject(t)
	sidecar := filepath.Join(project, ".nerv", "disabled.yaml")

	out, err := runVerb(t, "disable", "bindings", "alpha:PreToolUse", "--no-confirm")
	if err != nil || !strings.Contains(out, "disabled 1 binding(s):") || !strings.Contains(out, "alpha:PreToolUse") {
		t.Fatalf("disable bindings: err=%v out=\n%s", err, out)
	}
	if !strings.Contains(readFile(t, sidecar), "alpha") {
		t.Errorf("sidecar missing binding")
	}
	out, _ = runVerb(t, "disable", "bindings", "alpha:PreToolUse", "--no-confirm")
	if !strings.Contains(out, "no bindings changed (already disabled)") {
		t.Errorf("repeat should be a no-op:\n%s", out)
	}
	out, err = runVerb(t, "enable", "bindings", "alpha:PreToolUse", "--no-confirm")
	if err != nil || !strings.Contains(out, "enabled 1 binding(s):") {
		t.Fatalf("enable bindings: err=%v out=\n%s", err, out)
	}
	if _, err := runVerb(t, "disable", "bindings", "malformed", "--no-confirm"); err == nil {
		t.Error("malformed binding should error")
	}
}

func TestDisable_NeedsConfirmWithoutTTY(t *testing.T) {
	setupProject(t)
	// Stdin as /dev/null is a char device: stdinIsTTY would say true.
	// Use a pipe so the non-TTY refusal arm is the one exercised.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()
	orig := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = orig }()

	_, err = runVerb(t, "disable", "hooks", "alpha")
	if err == nil || !strings.Contains(err.Error(), "--no-confirm") {
		t.Errorf("want non-TTY refusal, got %v", err)
	}
	if y := readFile(t, ".nerv/hooks.yaml"); strings.Contains(y, "disabled") {
		t.Errorf("hooks.yaml mutated despite refusal:\n%s", y)
	}
}

func TestDisable_DryRunPlans(t *testing.T) {
	setupProject(t)
	setFormat(t, "json")
	before := readFile(t, ".nerv/hooks.yaml")
	for _, args := range [][]string{
		{"disable", "hooks", "alpha", "--dry-run"},
		{"disable", "events", "Stop", "--dry-run"},
		{"disable", "bindings", "alpha:Stop", "--dry-run"},
		{"enable", "hooks", "alpha", "--dry-run"},
	} {
		out, err := runVerb(t, args...)
		if err != nil {
			t.Errorf("%v: %v", args, err)
			continue
		}
		var plan planDoc
		if err := json.Unmarshal([]byte(out), &plan); err != nil {
			t.Errorf("%v: not a plan: %v\n%s", args, err, out)
		}
	}
	if after := readFile(t, ".nerv/hooks.yaml"); after != before {
		t.Errorf("dry-run changed hooks.yaml:\n%s", after)
	}
	if _, err := os.Stat(".nerv/disabled.yaml"); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote disabled.yaml: %v", err)
	}
}

func TestDisable_OutsideProjectErrors(t *testing.T) {
	isolateEnv(t) // no .nerv anywhere above the temp project
	if _, err := runVerb(t, "disable", "hooks", "alpha", "--no-confirm"); err == nil {
		t.Error("want error outside a nerv project")
	}
	if _, err := runVerb(t, "disable", "events", "Stop", "--no-confirm"); err == nil {
		t.Error("want error outside a nerv project")
	}
}

func TestStatus_RendersRows(t *testing.T) {
	setupProject(t)
	t.Setenv("PATH", "/usr/bin:/bin")
	setFormat(t, "json")
	out, err := runVerb(t, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	var rows []statusRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.Key] = r.Value
	}
	if got["nerv version"] != version {
		t.Errorf("version row = %q", got["nerv version"])
	}
	if got["adapters"] != "none detected" {
		t.Errorf("adapters row = %q", got["adapters"])
	}
	if got["hooks"] != "2 defined (version: 1)" || got["active events"] != "2 unique" {
		t.Errorf("hooks rows = %q / %q", got["hooks"], got["active events"])
	}
	if got["daemon"] != "stopped" || got["event log"] != "not created yet" {
		t.Errorf("daemon/eventlog rows = %q / %q", got["daemon"], got["event log"])
	}
}

func TestStatus_StalePIDAndAdapters(t *testing.T) {
	isolateEnv(t)
	bin := t.TempDir()
	writeExe(t, bin, "claude", "#!/bin/sh\n")
	writeExe(t, bin, "codex", "#!/bin/sh\n")
	t.Setenv("PATH", bin)
	setFormat(t, "json")

	pidfile, _ := daemonPaths()
	if err := os.MkdirAll(filepath.Dir(pidfile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidfile, []byte("2147483646\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(eventLogDir(), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runVerb(t, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "stopped (stale pid: 2147483646)") {
		t.Errorf("stale pid not reported:\n%s", out)
	}
	if strings.Contains(out, "none detected") || !strings.Contains(out, "claude") || !strings.Contains(out, "codex") {
		t.Errorf("adapters not listed:\n%s", out)
	}
	if strings.Contains(out, "not created yet") {
		t.Errorf("event log dir exists, should be reported:\n%s", out)
	}
}

func TestSplitPath(t *testing.T) {
	if got := splitPath(""); got != nil {
		t.Errorf("empty = %v", got)
	}
	got := splitPath("/a::/b")
	if len(got) != 3 || got[0] != "/a" || got[1] != "" || got[2] != "/b" {
		t.Errorf("split = %q", got)
	}
}
