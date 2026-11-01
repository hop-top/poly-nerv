//go:build !windows

package main

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/nerv/internal/config"
)

func writeExe(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheckAdapterCLI(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)

	c := checkAdapterCLI("claude", "claude")
	if c.Status != StatusSkip || c.Name != "adapter/claude" {
		t.Errorf("missing binary: %+v", c)
	}

	writeExe(t, bin, "claude", "#!/bin/sh\necho 'claude 9.9.9'\n")
	c = checkAdapterCLI("claude", "claude")
	if c.Status != StatusOK || c.Message != "claude 9.9.9" || c.Detail != filepath.Join(bin, "claude") {
		t.Errorf("working binary: %+v", c)
	}

	writeExe(t, bin, "codex", "#!/bin/sh\nexit 3\n")
	if c = checkAdapterCLI("codex", "codex"); c.Status != StatusWarn {
		t.Errorf("failing --version should WARN: %+v", c)
	}

	writeExe(t, bin, "gemini", "#!/bin/sh\nprintf '%0200d' 0\n")
	if c = checkAdapterCLI("gemini", "gemini"); c.Status != StatusOK || len(c.Message) != 80 {
		t.Errorf("version should be truncated to 80 chars: %+v (len %d)", c, len(c.Message))
	}
}

func TestCheckHookHandler(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	good := writeExe(t, dir, "good", "#!/bin/sh\n")
	noexec := filepath.Join(dir, "noexec")
	if err := os.WriteFile(noexec, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		run  string
		want CheckStatus
	}{
		{"empty run", "", StatusFail},
		{"missing binary", "nope --flag", StatusWarn},
		{"abs executable", good + " arg", StatusOK},
		{"abs non-executable", noexec, StatusFail},
		{"abs missing", filepath.Join(dir, "gone"), StatusWarn},
		{"rel non-executable", "./noexec", StatusFail},
		{"rel executable", "./good", StatusOK},
		// Bare name that is on PATH but not executable: not resolvable.
		{"bare non-executable", "noexec", StatusWarn},
	}
	t.Chdir(dir)
	for _, tc := range cases {
		c := checkHookHandler(config.HookEntry{Name: "h", Run: tc.run})
		if c.Status != tc.want || c.Name != "hook/h" {
			t.Errorf("%s: got %+v want status %s", tc.name, c, tc.want)
		}
	}
}

func TestCheckHookHandler_NotExecutableMessage(t *testing.T) {
	dir := t.TempDir()
	noexec := filepath.Join(dir, "noexec")
	if err := os.WriteFile(noexec, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := checkHookHandler(config.HookEntry{Name: "h", Run: noexec + " a"})
	if c.Status != StatusFail || !strings.Contains(c.Message, "not executable") {
		t.Errorf("got %+v", c)
	}
}

func TestCheckHooksConfig(t *testing.T) {
	project := isolateEnv(t)
	t.Setenv("PATH", "/usr/bin:/bin")

	// No hooks anywhere: WARN.
	cs := checkHooksConfig()
	if len(cs) != 1 || cs[0].Name != "hooks.yaml" || cs[0].Status != StatusWarn {
		t.Fatalf("no hooks: %+v", cs)
	}

	nervDir := filepath.Join(project, ".nerv")
	if err := os.MkdirAll(nervDir, 0o755); err != nil {
		t.Fatal(err)
	}
	h := writeExe(t, project, "h.sh", "#!/bin/sh\n")
	yaml := "version: \"1\"\nhooks:\n  - name: a\n    on: PreToolUse\n    run: " + h + "\n"
	if err := os.WriteFile(filepath.Join(nervDir, "hooks.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cs = checkHooksConfig()
	if len(cs) != 2 || cs[0].Status != StatusOK || !strings.Contains(cs[0].Message, "1 hook(s)") ||
		cs[1].Name != "hook/a" || cs[1].Status != StatusOK {
		t.Fatalf("one good hook: %+v", cs)
	}

	// Broken YAML: FAIL.
	if err := os.WriteFile(filepath.Join(nervDir, "hooks.yaml"), []byte("hooks: [unterminated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cs = checkHooksConfig()
	if len(cs) != 1 || cs[0].Status != StatusFail {
		t.Fatalf("broken yaml: %+v", cs)
	}
}

func TestCheckEventLog(t *testing.T) {
	isolateEnv(t)

	c := checkEventLog()
	if c.Status != StatusWarn || !strings.Contains(c.Message, "does not exist") {
		t.Errorf("missing dir: %+v", c)
	}

	dir := eventLogDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	c = checkEventLog()
	if c.Status != StatusOK || !strings.Contains(c.Message, "no entries") {
		t.Errorf("empty dir: %+v", c)
	}
	if _, err := os.Stat(filepath.Join(dir, ".nerv-doctor-probe")); err == nil {
		t.Error("probe file not cleaned up")
	}

	// A file where the dir should be.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c = checkEventLog(); c.Status != StatusFail || !strings.Contains(c.Message, "not a directory") {
		t.Errorf("file in place of dir: %+v", c)
	}

	// Read-only dir (skip as root, which ignores modes).
	if os.Geteuid() != 0 {
		_ = os.Remove(dir)
		if err := os.MkdirAll(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
		if c = checkEventLog(); c.Status != StatusFail || !strings.Contains(c.Message, "not writable") {
			t.Errorf("read-only dir: %+v", c)
		}
	}
}

func TestCheckDaemon(t *testing.T) {
	isolateEnv(t)
	// Short runtime dir: unix socket paths are length-limited.
	run, err := os.MkdirTemp("/tmp", "nrv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(run) })
	t.Setenv("XDG_RUNTIME_DIR", run)

	if c := checkDaemon(); c.Status != StatusSkip {
		t.Errorf("no socket: %+v", c)
	}

	// Stale socket file (regular file) -> dial fails.
	sock := filepath.Join(run, "nerv.sock")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if c := checkDaemon(); c.Status != StatusFail {
		t.Errorf("stale socket: %+v", c)
	}
	_ = os.Remove(sock)

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix listen unavailable: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if checkDaemon().Status == StatusOK {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("live socket not reported OK: %+v", checkDaemon())
}

func TestRunDoctor_JSON(t *testing.T) {
	isolateEnv(t)
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	writeExe(t, bin, "claude", "#!/bin/sh\necho v1\n")
	setFormat(t, "json")

	var runErr error
	out := captureStdout(t, func() { runErr = runDoctor(doctorCmd(), nil) })
	if runErr != nil {
		t.Fatalf("runDoctor: %v", runErr)
	}
	var checks []Check
	if err := json.Unmarshal([]byte(out), &checks); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	got := map[string]CheckStatus{}
	for _, c := range checks {
		got[c.Name] = c.Status
	}
	want := map[string]CheckStatus{
		"adapter/claude":   StatusOK,
		"adapter/codex":    StatusSkip,
		"adapter/gemini":   StatusSkip,
		"adapter/opencode": StatusSkip,
		"hooks.yaml":       StatusWarn,
		"eventlog":         StatusWarn,
		"daemon":           StatusSkip,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("check %s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
}
