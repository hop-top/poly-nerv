//go:build !windows

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

	"github.com/spf13/cobra"
)

func runDaemonVerb(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "nerv", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.AddCommand(daemonCmd())
	root.SetArgs(append([]string{"daemon"}, args...))
	var err error
	out := captureStdout(t, func() { err = root.Execute() })
	return out, err
}

// spawnSleeper starts a harmless child, reaps it in the background,
// and writes its pid into the daemon pidfile. Returns the pid and a
// channel that closes when the child has exited.
func spawnSleeper(t *testing.T) (int, <-chan error) {
	t.Helper()
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	t.Cleanup(func() { _ = child.Process.Kill() })
	pidfile, _ := daemonPaths()
	if err := os.MkdirAll(filepath.Dir(pidfile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pidfile, []byte(strconv.Itoa(child.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}
	return child.Process.Pid, done
}

func TestReadPID(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pid")
	if _, err := readPID(p); err == nil {
		t.Error("missing file should error")
	}
	_ = os.WriteFile(p, []byte("abc"), 0o644)
	if _, err := readPID(p); err == nil || !strings.Contains(err.Error(), "invalid pid") {
		t.Errorf("garbage pid: %v", err)
	}
	_ = os.WriteFile(p, []byte(" 123\n"), 0o644)
	if pid, err := readPID(p); err != nil || pid != 123 {
		t.Errorf("pid = %d err=%v", pid, err)
	}
}

func TestDaemonStatus_States(t *testing.T) {
	isolateEnv(t)
	setFormat(t, "json")

	state := func() daemonStatusRow {
		out, err := runDaemonVerb(t, "status")
		if err != nil {
			t.Fatalf("status: %v", err)
		}
		var row daemonStatusRow
		if err := json.Unmarshal([]byte(out), &row); err != nil {
			t.Fatalf("not JSON: %v\n%s", err, out)
		}
		return row
	}

	if r := state(); r.State != "stopped" || r.PID != 0 {
		t.Errorf("no pidfile: %+v", r)
	}

	pidfile, _ := daemonPaths()
	_ = os.MkdirAll(filepath.Dir(pidfile), 0o755)
	_ = os.WriteFile(pidfile, []byte("2147483646"), 0o644)
	if r := state(); r.State != "stopped (stale pidfile)" || r.PID != 2147483646 {
		t.Errorf("stale pid: %+v", r)
	}

	pid, _ := spawnSleeper(t)
	if r := state(); r.State != "running" || r.PID != pid || r.Socket != "unavailable" {
		t.Errorf("live pid, no socket: %+v", r)
	}
}

func TestDaemonStop_TerminatesProcessAndCleansUp(t *testing.T) {
	isolateEnv(t)
	pid, done := spawnSleeper(t)
	pidfile, sock := daemonPaths()
	_ = os.MkdirAll(filepath.Dir(sock), 0o755)
	_ = os.WriteFile(sock, nil, 0o644)

	out, err := runDaemonVerb(t, "stop")
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if strings.TrimSpace(out) != "daemon stopped" {
		t.Errorf("out = %q", out)
	}
	select {
	case werr := <-done:
		if ee, ok := werr.(*exec.ExitError); !ok || ee.Sys().(syscall.WaitStatus).Signal() != syscall.SIGTERM {
			t.Errorf("child (pid %d) should die of SIGTERM, got %v", pid, werr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child still alive after stop")
	}
	for _, p := range []string{pidfile, sock} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s not cleaned up: %v", p, err)
		}
	}
}

func TestDaemonStop_NoPidfile(t *testing.T) {
	isolateEnv(t)
	_, err := runDaemonVerb(t, "stop")
	if err == nil || !strings.Contains(err.Error(), "daemon not running") {
		t.Errorf("err = %v", err)
	}
}

func TestDaemonStop_StalePidCleansUp(t *testing.T) {
	isolateEnv(t)
	pidfile, _ := daemonPaths()
	_ = os.MkdirAll(filepath.Dir(pidfile), 0o755)
	_ = os.WriteFile(pidfile, []byte("2147483646"), 0o644)
	out, err := runDaemonVerb(t, "stop")
	if err != nil || !strings.Contains(out, "daemon stopped") {
		t.Errorf("err=%v out=%q", err, out)
	}
	if _, err := os.Stat(pidfile); !os.IsNotExist(err) {
		t.Errorf("stale pidfile not removed: %v", err)
	}
}

func TestDaemonReload(t *testing.T) {
	isolateEnv(t)
	if _, err := runDaemonVerb(t, "reload"); err == nil || !strings.Contains(err.Error(), "no pidfile") {
		t.Errorf("no pidfile: %v", err)
	}
	pidfile, _ := daemonPaths()
	_ = os.MkdirAll(filepath.Dir(pidfile), 0o755)
	_ = os.WriteFile(pidfile, []byte("2147483646"), 0o644)
	if _, err := runDaemonVerb(t, "reload"); err == nil || !strings.Contains(err.Error(), "stale pidfile") {
		t.Errorf("stale: %v", err)
	}

	pid, done := spawnSleeper(t)
	out, err := runDaemonVerb(t, "reload")
	if err != nil || !strings.Contains(out, "reload signal sent (pid: "+strconv.Itoa(pid)+")") {
		t.Fatalf("reload: err=%v out=%q", err, out)
	}
	// sleep has no SIGHUP handler, so delivery is observable.
	select {
	case werr := <-done:
		if ee, ok := werr.(*exec.ExitError); !ok || ee.Sys().(syscall.WaitStatus).Signal() != syscall.SIGHUP {
			t.Errorf("child should die of SIGHUP, got %v", werr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SIGHUP not delivered")
	}
}

func TestDaemon_DryRunSendsNoSignals(t *testing.T) {
	isolateEnv(t)
	setFormat(t, "json")

	// Not running: plan has warnings, no effects.
	for _, verb := range []string{"stop", "reload"} {
		out, err := runDaemonVerb(t, verb, "--dry-run")
		if err != nil {
			t.Fatalf("%s --dry-run: %v", verb, err)
		}
		if !strings.Contains(out, "daemon is not running") {
			t.Errorf("%s plan lacks warning:\n%s", verb, out)
		}
	}

	// Running: plan lists a signal effect and the child survives.
	pid, done := spawnSleeper(t)
	for verb, wantKinds := range map[string]int{"stop": 3, "reload": 1} {
		out, err := runDaemonVerb(t, verb, "--dry-run")
		if err != nil {
			t.Fatalf("%s --dry-run: %v", verb, err)
		}
		var plan planDoc
		if err := json.Unmarshal([]byte(out), &plan); err != nil {
			t.Fatalf("%s: not a plan: %v\n%s", verb, err, out)
		}
		if len(plan.Effects) != wantKinds || plan.Effects[0].Kind != "signal" ||
			plan.Effects[0].Target != "pid:"+strconv.Itoa(pid) {
			t.Errorf("%s plan = %+v", verb, plan)
		}
	}
	select {
	case werr := <-done:
		t.Fatalf("dry-run killed the child: %v", werr)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestDaemonStart(t *testing.T) {
	isolateEnv(t)
	bin := t.TempDir()
	t.Setenv("PATH", "/usr/bin:/bin:"+bin)

	// A nervd that is not on PATH nor beside the test binary.
	if _, err := runDaemonVerb(t, "start"); err == nil || !strings.Contains(err.Error(), "nervd binary not found") {
		t.Fatalf("missing nervd: %v", err)
	}

	writeExe(t, bin, "nervd", "#!/bin/sh\nexec sleep 60\n")
	out, err := runDaemonVerb(t, "start")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	pidfile, _ := daemonPaths()
	pid, rerr := readPID(pidfile)
	if rerr != nil {
		t.Fatalf("pidfile: %v", rerr)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	if !strings.Contains(out, "daemon started (pid: "+strconv.Itoa(pid)+")") {
		t.Errorf("out = %q", out)
	}
	if !processAlive(pid) {
		t.Error("started daemon not alive")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(pidfile), "daemon.log")); err != nil {
		t.Errorf("daemon.log missing: %v", err)
	}

	// Second start refuses to spawn a duplicate.
	if _, err := runDaemonVerb(t, "start"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Errorf("duplicate start: %v", err)
	}
}
