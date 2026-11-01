package main

import (
	"errors"
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

func TestFindHookLine_StructureNotKeyName(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want []int
	}{
		{"name first", "hooks:\n  - name: a\n    run: x\n  - name: b\n", []int{2, 4}},
		{"on first", "hooks:\n  - on: PreToolUse\n    name: a\n  - on: Stop\n    name: b\n", []int{2, 4}},
		{"run first", "hooks:\n  - run: x\n    name: a\n  - run: y\n", []int{2, 4}},
		{"mixed orders", "hooks:\n  - name: a\n  - on: Stop\n  - run: z\n  - name: d\n", []int{2, 3, 4, 5}},
		{"comments and blanks", "# top\nversion: \"1\"\n\nhooks:\n  # first\n\n  - on: Stop\n    # inner\n    name: a\n\n  # between\n  - run: y\n", []int{7, 12}},
		{"flow style", "hooks:\n  - {on: Stop, name: a}\n  - {name: b}\n  - on: Stop\n", []int{2, 3, 4}},
		{"name key nested in value", "hooks:\n  - on: Stop\n    env:\n      - name: nested\n  - on: Stop\n", []int{2, 5}},
		{"hooks not first key", "version: \"1\"\ndefaults: {}\nhooks:\n  - run: x\n", []int{4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i, want := range tc.want {
				if got := findHookLine([]byte(tc.yaml), i); got != want {
					t.Errorf("hook %d line = %d, want %d", i, got, want)
				}
			}
			if got := findHookLine([]byte(tc.yaml), len(tc.want)); got != 0 {
				t.Errorf("out-of-range = %d, want 0", got)
			}
		})
	}
}

func TestLintFile_LineNumbersWithOnFirst(t *testing.T) {
	got := lintFile(writeLintYAML(t, "version: \"1\"\nhooks:\n  - on: PreToolUse\n    run: /bin/true\n  - on: Bogus\n    name: b\n    run: /bin/true\n"))
	iss := findIssue(got, "error", `unknown event "Bogus"`)
	if iss == nil || iss.Line != 5 {
		t.Errorf("want unknown-event at line 5: %+v", got)
	}
	iss = findIssue(got, "error", "missing 'name'")
	if iss == nil || iss.Line != 3 {
		t.Errorf("want missing-name at line 3: %+v", got)
	}
}

func TestRunLint_ErrorSeverityReturnsTypedError(t *testing.T) {
	isolateEnv(t)
	t.Setenv("PATH", "/usr/bin:/bin")
	bad := writeLintYAML(t, "version: \"1\"\nhooks:\n  - name: h\n    on: Bogus\n    run: /bin/true\n")

	for _, format := range []string{"", "json"} {
		setFormat(t, format)
		cmd := lintCmd()
		var stderr strings.Builder
		cmd.SetErr(&stderr)
		var err error
		out := captureStdout(t, func() { err = runLint(cmd, []string{bad}) })
		if !errors.Is(err, errLintFailed) {
			t.Fatalf("format %q: err = %v, want errLintFailed", format, err)
		}
		if got := exitCodeFor(err); got != 1 {
			t.Errorf("format %q: exitCodeFor = %d, want 1", format, got)
		}
		if !strings.Contains(out, "unknown event") {
			t.Errorf("format %q: issues not rendered on stdout: %q", format, out)
		}
		if stderr.Len() != 0 {
			t.Errorf("format %q: stderr not empty: %q", format, stderr.String())
		}
	}
}

// TestLintBinary_ErrorSeverityExitsOneQuietStderr pins the observable
// contract through the real CLI path: exit 1, issues on stdout in both
// formats, nothing on stderr.
func TestLintBinary_ErrorSeverityExitsOneQuietStderr(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()
	bad := writeLintYAML(t, "version: \"1\"\nhooks:\n  - on: Bogus\n    run: /bin/true\n")
	for _, format := range []string{"table", "json"} {
		stdout, stderr, code := runInProjectEnv(t, bin, project,
			[]string{"HOME=" + project}, "lint", bad, "--format", format)
		if code != 1 {
			t.Errorf("%s: exit %d, want 1", format, code)
		}
		if !strings.Contains(stdout, "unknown event") {
			t.Errorf("%s: stdout missing issues:\n%s", format, stdout)
		}
		if stderr != "" {
			t.Errorf("%s: stderr not empty:\n%s", format, stderr)
		}
	}
}
