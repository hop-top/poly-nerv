package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLintYAML(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hooks.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func findIssue(issues []lintIssue, sev, sub string) *lintIssue {
	for i := range issues {
		if issues[i].Severity == sev && strings.Contains(issues[i].Message, sub) {
			return &issues[i]
		}
	}
	return nil
}

func TestLintFile_MissingFileIsFine(t *testing.T) {
	if got := lintFile(filepath.Join(t.TempDir(), "nope.yaml")); got != nil {
		t.Errorf("missing layer should yield no issues: %+v", got)
	}
}

func TestLintFile_UnreadablePath(t *testing.T) {
	// A directory read fails with a non-NotExist error.
	got := lintFile(t.TempDir())
	if findIssue(got, "error", "cannot read") == nil {
		t.Errorf("want cannot-read error: %+v", got)
	}
}

func TestLintFile_InvalidYAML(t *testing.T) {
	got := lintFile(writeLintYAML(t, "hooks: [unterminated\n"))
	iss := findIssue(got, "error", "invalid YAML")
	if iss == nil || iss.Line != 1 {
		t.Errorf("want invalid-YAML error at line 1: %+v", got)
	}
}

func TestLintFile_SchemaMismatch(t *testing.T) {
	got := lintFile(writeLintYAML(t, "version: \"1\"\nhooks: not-a-list\n"))
	if findIssue(got, "error", "schema mismatch") == nil {
		t.Errorf("want schema-mismatch error: %+v", got)
	}
}

func TestLintFile_MissingVersionAndFields(t *testing.T) {
	got := lintFile(writeLintYAML(t, `hooks:
  - on: PreToolUse
    run: /bin/true
  - name: noon
    run: /bin/true
  - name: badev
    on: NotAnEvent
    run: /bin/true
  - name: norun
    on: PreToolUse
`))
	if findIssue(got, "warning", "missing 'version'") == nil {
		t.Errorf("missing version warning absent: %+v", got)
	}
	if iss := findIssue(got, "error", "missing 'name'"); iss == nil || iss.Hook != "hooks[0]" {
		t.Errorf("missing name error wrong: %+v / %+v", iss, got)
	}
	if iss := findIssue(got, "error", "missing 'on'"); iss == nil || iss.Hook != "noon" {
		t.Errorf("missing on error wrong: %+v", got)
	}
	if iss := findIssue(got, "error", `unknown event "NotAnEvent"`); iss == nil || iss.Hook != "badev" {
		t.Errorf("unknown event error wrong: %+v", got)
	}
	if iss := findIssue(got, "error", "missing 'run'"); iss == nil || iss.Hook != "norun" {
		t.Errorf("missing run error wrong: %+v", got)
	}
}

func TestLintHandler_Variants(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", "/usr/bin:/bin:"+dir)
	ok := writeExe(t, dir, "ok.sh", "#!/bin/sh\necho '{}'\n")
	exit9 := writeExe(t, dir, "exit9.sh", "#!/bin/sh\nexit 9\n")
	exit2 := writeExe(t, dir, "exit2.sh", "#!/bin/sh\nexit 2\n")
	badjson := writeExe(t, dir, "badjson.sh", "#!/bin/sh\necho notjson\n")
	noexec := filepath.Join(dir, "noexec.sh")
	if err := os.WriteFile(noexec, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mk := func(run string) []lintIssue {
		return lintFile(writeLintYAML(t, "version: \"1\"\nhooks:\n  - name: h\n    on: PreToolUse\n    run: "+run+"\n"))
	}

	if got := mk(ok); len(got) != 0 {
		t.Errorf("clean handler produced issues: %+v", got)
	}
	if got := mk(exit2); len(got) != 0 {
		t.Errorf("exit 2 is within 0-3, got issues: %+v", got)
	}
	if got := mk(exit9); findIssue(got, "warning", "exited 9") == nil {
		t.Errorf("exit 9 should warn: %+v", got)
	}
	if got := mk(badjson); findIssue(got, "warning", "not valid JSON") == nil {
		t.Errorf("non-JSON stdout should warn: %+v", got)
	}
	if got := mk(noexec); findIssue(got, "error", "not executable") == nil {
		t.Errorf("non-executable should error: %+v", got)
	}
	if got := mk("/definitely/not/here.sh"); findIssue(got, "error", "not found") == nil {
		t.Errorf("absolute missing path should error: %+v", got)
	}
	if got := mk("./no-such-relative.sh"); findIssue(got, "error", "not found") == nil {
		t.Errorf("relative missing path should error: %+v", got)
	}
	if got := mk("no-such-binary-xyz"); findIssue(got, "error", "not on PATH") == nil {
		t.Errorf("bare missing binary should error: %+v", got)
	}
}

func TestFindHookLine(t *testing.T) {
	data := []byte("version: 1\nhooks:\n  - name: a\n    run: x\n  - name: b\n")
	if got := findHookLine(data, 0); got != 3 {
		t.Errorf("hook 0 line = %d, want 3", got)
	}
	if got := findHookLine(data, 1); got != 5 {
		t.Errorf("hook 1 line = %d, want 5", got)
	}
	if got := findHookLine(data, 2); got != 0 {
		t.Errorf("out-of-range hook line = %d, want 0", got)
	}
}

func TestRunLint_Output(t *testing.T) {
	isolateEnv(t)
	t.Setenv("PATH", "/usr/bin:/bin")

	clean := writeLintYAML(t, "version: \"1\"\nhooks: []\n")
	setFormat(t, "")
	var err error
	out := captureStdout(t, func() { err = runLint(lintCmd(), []string{clean}) })
	if err != nil || strings.TrimSpace(out) != "No issues found." {
		t.Errorf("clean: err=%v out=%q", err, out)
	}

	// JSON clean run prints nothing, still succeeds.
	setFormat(t, "json")
	out = captureStdout(t, func() { err = runLint(lintCmd(), []string{clean}) })
	if err != nil || out != "" {
		t.Errorf("json clean: err=%v out=%q", err, out)
	}

	// Warning-only file renders issues and returns nil (no os.Exit).
	warn := writeLintYAML(t, "hooks: []\n")
	out = captureStdout(t, func() { err = runLint(lintCmd(), []string{warn}) })
	if err != nil || !strings.Contains(out, "missing 'version'") {
		t.Errorf("warn: err=%v out=%q", err, out)
	}

	// No arg: scans layer paths; isolated env has none -> clean.
	setFormat(t, "")
	out = captureStdout(t, func() { err = runLint(lintCmd(), nil) })
	if err != nil || !strings.Contains(out, "No issues found.") {
		t.Errorf("layers: err=%v out=%q", err, out)
	}
}
