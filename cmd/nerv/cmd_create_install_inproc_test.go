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

func runRootVerb(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "nerv", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("dry-run", false, "")
	install := installCmd()
	install.AddCommand(installAddCmd(), installStatusCmd())
	root.AddCommand(createCmd(), install)
	root.SetArgs(args)
	var err error
	out := captureStdout(t, func() { err = root.Execute() })
	return out, err
}

func TestCreate_DryRunPlan(t *testing.T) {
	project := isolateEnv(t)
	setFormat(t, "json")
	out, err := runRootVerb(t, "create", "block rm -rf commands", "--dry-run", "--name", "Guard", "--events", "PreToolUse")
	if err != nil {
		t.Fatalf("create --dry-run: %v", err)
	}
	var plan struct {
		Command  string         `json:"command"`
		Args     map[string]any `json:"args"`
		Effects  []struct{ Kind, Target, Detail string }
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("not a plan: %v\n%s", err, out)
	}
	if plan.Args["hook"] != "guard" || plan.Args["lang"] != "sh" {
		t.Errorf("args = %v (name override must be lower-cased)", plan.Args)
	}
	if len(plan.Effects) != 3 || !strings.Contains(plan.Effects[1].Detail, "on: PreToolUse") {
		t.Errorf("effects = %+v", plan.Effects)
	}
	// Explicit name: no "derived" warning.
	if len(plan.Warnings) != 1 {
		t.Errorf("warnings = %v", plan.Warnings)
	}
	if _, err := os.Stat(filepath.Join(project, ".nerv")); !os.IsNotExist(err) {
		t.Errorf("dry-run created .nerv: %v", err)
	}

	// Derived name adds a warning and default event text.
	out, err = runRootVerb(t, "create", "log every tool call", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "is derived from the intent") || !strings.Contains(out, "inferred by the provider") {
		t.Errorf("derived-name plan:\n%s", out)
	}
}

func TestCreate_RejectsBadInput(t *testing.T) {
	isolateEnv(t)
	setFormat(t, "json")
	if _, err := runRootVerb(t, "create", "x", "--lang", "cobol", "--dry-run"); err == nil {
		t.Error("unsupported --lang must fail")
	}
	_, err := runRootVerb(t, "create", "x", "--events", "NotAnEvent", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "unknown event") {
		t.Errorf("unknown event: %v", err)
	}
	if _, err := runRootVerb(t, "create"); err == nil {
		t.Error("missing description must fail")
	}
}

func TestCreate_UnresolvableProviderFails(t *testing.T) {
	project := isolateEnv(t)
	t.Setenv("LLM_PROVIDER", "")
	_, err := runRootVerb(t, "create", "do a thing", "--llm-provider", "nosuchscheme://x")
	if err == nil || !strings.Contains(err.Error(), "create:") {
		t.Errorf("want wrapped provider error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".nerv")); !os.IsNotExist(statErr) {
		t.Errorf("failed create left .nerv behind: %v", statErr)
	}
}

func TestInstallAdd_UnknownAdapter(t *testing.T) {
	isolateEnv(t)
	_, err := runRootVerb(t, "install", "add", "no-such-cli")
	if err == nil || !strings.Contains(err.Error(), `no nerv adapter for "no-such-cli"`) {
		t.Errorf("err = %v", err)
	}
}

func TestInstallAdd_NothingDetected(t *testing.T) {
	isolateEnv(t)
	t.Setenv("PATH", t.TempDir())
	out, err := runRootVerb(t, "install", "add")
	if err != nil {
		t.Fatalf("install add: %v", err)
	}
	if !strings.Contains(out, "installed 0 adapter(s), skipped") {
		t.Errorf("out = %q", out)
	}
}

func TestInstallAdd_DryRunPlan(t *testing.T) {
	isolateEnv(t)
	// Detection must not depend on a real claude on the host PATH.
	bin := t.TempDir()
	stub := filepath.Join(bin, "claude")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho 1.2.1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	setFormat(t, "json")
	out, err := runRootVerb(t, "install", "add", "claude", "--dry-run")
	if err != nil {
		t.Fatalf("install add --dry-run: %v", err)
	}
	var plan planDoc
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("not a plan: %v\n%s", err, out)
	}
	if len(plan.Effects) == 0 {
		t.Errorf("plan has no effects:\n%s", out)
	}
}
