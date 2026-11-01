//go:build !windows

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/dispatch"
)

// failReader fails the test if anything reads it: Aider shares its
// terminal stdin with lint-cmd/test-cmd, so nervd must never touch it.
type failReader struct{ t *testing.T }

func (f failReader) Read([]byte) (int, error) {
	f.t.Helper()
	f.t.Error("aider route read stdin")
	return 0, errors.New("stdin read")
}

func aiderRun(t *testing.T, d *dispatch.Dispatcher, args ...string) (string, string, int) {
	t.Helper()
	route, err := lookupAdapter("aider")
	if err != nil || route == nil {
		t.Fatalf("lookupAdapter(aider): %v %v", route, err)
	}
	var out, errw bytes.Buffer
	code := runStatelessArgs(d, route, args, failReader{t}, &out, &errw)
	return out.String(), errw.String(), code
}

// TestAdapterAider_Semantics: block -> exit 1 + message on stdout (what
// Aider feeds the model); everything else -> exit 0, silent stdout.
func TestAdapterAider_Semantics(t *testing.T) {
	cases := []struct {
		name, nervEvent, body string
		args                  []string
		hookExit              int
		wantOut               string
		wantCode              int
		wantErr               string
	}{
		{"lint block", "FileChanged", `{"action":"block","message":"no secrets in a.py","context":"x"}`,
			[]string{"lint", "a.py"}, 2, "no secrets in a.py", 1, ""},
		{"test block", "PostToolUse", `{"action":"block","message":"tests gated"}`,
			[]string{"test"}, 2, "tests gated", 1, ""},
		{"block no message", "FileChanged", `{"action":"block"}`,
			[]string{"lint", "a.py"}, 2, "blocked by nerv", 1, ""},
		{"lint allow", "FileChanged", `{"action":"allow"}`,
			[]string{"lint", "a.py"}, 0, "", 0, ""},
		{"warn is not a failure", "FileChanged", `{"action":"warn","message":"careful"}`,
			[]string{"lint", "a.py"}, 1, "", 0, "careful"},
		{"context dropped", "PostToolUse", `{"action":"allow","context":"impact: 3"}`,
			[]string{"test"}, 0, "", 0, ""},
		{"rewrite dropped", "FileChanged", `{"action":"allow","rewrite":{"x":1}}`,
			[]string{"lint", "a.py"}, 0, "", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := ctxDispatcher(t, tc.nervEvent, tc.body, tc.hookExit)
			out, errw, code := aiderRun(t, d, tc.args...)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d (stderr %q)", code, tc.wantCode, errw)
			}
			if strings.TrimSpace(out) != tc.wantOut {
				t.Fatalf("stdout = %q, want %q", out, tc.wantOut)
			}
			if !strings.Contains(errw, tc.wantErr) {
				t.Fatalf("stderr = %q, want contains %q", errw, tc.wantErr)
			}
		})
	}
}

// TestAdapterAider_EventMapping: lint fires FileChanged with every
// appended file (spaces survive); test fires PostToolUse. A hook on the
// other event must not run.
func TestAdapterAider_EventMapping(t *testing.T) {
	dir := t.TempDir()
	rec := filepath.Join(dir, "rec")
	script := filepath.Join(dir, "h.sh")
	src := "#!/bin/sh\ncat >> " + rec + "\necho >> " + rec + "\necho '{\"action\":\"allow\"}'\n"
	if err := os.WriteFile(script, []byte(src), 0o700); err != nil {
		t.Fatal(err)
	}
	cc := config.Compile(&config.HooksConfig{Hooks: []config.HookEntry{
		{Name: "fc", On: "FileChanged", Run: script},
	}})
	d := dispatch.New(cc, nil, dispatch.NewMatcher(), dispatch.NewRunner())

	_, _, code := aiderRun(t, d, "lint", "dir with space/a b.py", "c.py")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	b, _ := os.ReadFile(rec)
	for _, want := range []string{`dir with space/a b.py`, `c.py`, `FileChanged`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("hook input missing %q:\n%s", want, b)
		}
	}

	if err := os.Remove(rec); err != nil {
		t.Fatal(err)
	}
	if _, _, code := aiderRun(t, d, "test"); code != 0 {
		t.Fatalf("test exit %d", code)
	}
	if _, err := os.Stat(rec); err == nil {
		t.Fatal("FileChanged hook ran for test-cmd")
	}
}

func TestAdapterAider_BadArgs(t *testing.T) {
	d := ctxDispatcher(t, "FileChanged", `{"action":"allow"}`, 0)
	for _, args := range [][]string{nil, {"bogus"}} {
		if _, errw, code := aiderRun(t, d, args...); code != 2 || !strings.Contains(errw, "aider:") {
			t.Fatalf("args %v: exit %d stderr %q", args, code, errw)
		}
	}
}
