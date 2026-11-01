package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func todayLog(t *testing.T) string {
	t.Helper()
	dir := eventLogDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "events-"+time.Now().Format("2006-01-02")+".jsonl")
}

func appendEntry(t *testing.T, path, event string) {
	t.Helper()
	b, err := json.Marshal(tailEntry(event, "claude", 5, "allow"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(b, '\n')); err != nil {
		t.Fatal(err)
	}
}

// runTailCtx runs runTail in-process with ctx and returns stdout.
func runTailCtx(t *testing.T, ctx context.Context, args ...string) (string, error) {
	t.Helper()
	cmd := tailCmd()
	cmd.SetContext(ctx)
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	var err error
	out := captureStdout(t, func() { err = runTail(cmd, nil) })
	return out, err
}

func TestRunTail_NoFollowPrintsAndReturns(t *testing.T) {
	isolateEnv(t)
	setFormat(t, "table")
	p := todayLog(t)
	appendEntry(t, p, "PreToolUse")
	appendEntry(t, p, "PostToolUse")

	done := make(chan struct{})
	var out string
	var err error
	go func() {
		out, err = runTailCtx(t, context.Background(), "--no-follow")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("--no-follow did not return")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "PreToolUse") || !strings.Contains(out, "PostToolUse") {
		t.Errorf("output missing entries: %q", out)
	}
}

func TestRunTail_NoFollowEmptyAndMissing(t *testing.T) {
	isolateEnv(t)
	setFormat(t, "table")

	// Missing log dir.
	out, err := runTailCtx(t, context.Background(), "--no-follow")
	if err != nil || out != "" {
		t.Errorf("missing log: out=%q err=%v", out, err)
	}

	// Empty log file.
	if err := os.WriteFile(todayLog(t), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = runTailCtx(t, context.Background(), "--no-follow")
	if err != nil || out != "" {
		t.Errorf("empty log: out=%q err=%v", out, err)
	}
}

func TestRunTail_FollowReturnsOnCancelMissingLog(t *testing.T) {
	isolateEnv(t)
	setFormat(t, "table")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := runTailCtx(t, ctx)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follow did not return after cancel")
	}
}

func TestRunTail_FollowPicksUpAppendedThenCancels(t *testing.T) {
	isolateEnv(t)
	setFormat(t, "table")
	p := todayLog(t)
	appendEntry(t, p, "Seed")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopApp := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopApp:
				return
			case <-time.After(50 * time.Millisecond):
				appendEntry(t, p, "Appended")
			}
		}
	}()
	defer close(stopApp)

	done := make(chan struct{})
	var out string
	var err error
	go func() {
		out, err = runTailCtx(t, ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("follow did not return after cancel")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Seed") || !strings.Contains(out, "Appended") {
		t.Errorf("want initial + appended entries, got %q", out)
	}
}
