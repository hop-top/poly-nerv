package create_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/kit/go/ai/llm"

	"hop.top/nerv/internal/hookpkg"
	"hop.top/nerv/internal/hookpkg/create"
)

// fakeCompleter is a deterministic llm.Completer used to drive the
// kitLLMGenerator without any real network calls.
type fakeCompleter struct {
	resp llm.Response
	err  error
	last llm.Request
}

func (f *fakeCompleter) Close() error { return nil }

func (f *fakeCompleter) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	f.last = req
	return f.resp, f.err
}

// jsonResp builds the strict-JSON response shape the prompt asks for,
// wrapped as an llm.Response.
func jsonResp(name, script, rationale string, events []string) llm.Response {
	body, _ := json.Marshal(map[string]any{
		"name":      name,
		"events":    events,
		"script":    script,
		"rationale": rationale,
	})
	return llm.Response{Content: string(body)}
}

// stubGenerator is a Generator with a canned Result. Used to test the
// orchestration path independent of any LLM transport.
type stubGenerator struct {
	res *create.Result
	err error
}

func (s *stubGenerator) Generate(
	_ context.Context, _, _ string, _ []string,
) (*create.Result, error) {
	if s.err != nil {
		return nil, s.err
	}
	// Return a copy so tests can mutate independently.
	cp := *s.res
	return &cp, nil
}

func TestKitLLMGenerator_ParsesStrictJSON(t *testing.T) {
	fc := &fakeCompleter{
		resp: jsonResp(
			"warn-large-writes",
			"#!/bin/sh\necho '{\"action\":\"WARN\"}'\n",
			"warn on >1MB writes",
			[]string{"PreToolUse"},
		),
	}
	g := create.NewGeneratorFromCompleter(fc, "test-model")

	res, err := g.Generate(context.Background(),
		"warn when files >1MB are written", "sh", nil)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.Name != "warn-large-writes" {
		t.Errorf("Name = %q, want warn-large-writes", res.Name)
	}
	if res.Lang != "sh" {
		t.Errorf("Lang = %q, want sh", res.Lang)
	}
	if !strings.Contains(res.Script, "#!/bin/sh") {
		t.Errorf("Script missing shebang: %q", res.Script)
	}
	if len(res.Events) != 1 || res.Events[0] != "PreToolUse" {
		t.Errorf("Events = %v, want [PreToolUse]", res.Events)
	}
	if fc.last.Model != "test-model" {
		t.Errorf("Request.Model = %q, want test-model", fc.last.Model)
	}
	// The prompt MUST include the intent.
	if !strings.Contains(fc.last.Messages[0].Content, "warn when files >1MB") {
		t.Errorf("prompt missing intent: %q", fc.last.Messages[0].Content)
	}
}

func TestKitLLMGenerator_HonoursEventsOverride(t *testing.T) {
	fc := &fakeCompleter{
		// Model returns a different event list — override must win.
		resp: jsonResp(
			"audit", "#!/bin/sh\nexit 0\n", "audit", []string{"PreToolUse"},
		),
	}
	g := create.NewGeneratorFromCompleter(fc, "m")

	res, err := g.Generate(context.Background(), "audit everything",
		"sh", []string{"PostToolUse", "Stop"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	want := []string{"PostToolUse", "Stop"}
	if !equalSlices(res.Events, want) {
		t.Errorf("Events = %v, want %v", res.Events, want)
	}
}

func TestKitLLMGenerator_RejectsEmptyIntent(t *testing.T) {
	fc := &fakeCompleter{}
	g := create.NewGeneratorFromCompleter(fc, "m")

	_, err := g.Generate(context.Background(), "   ", "sh", nil)
	if !errors.Is(err, create.ErrEmptyIntent) {
		t.Errorf("err = %v, want ErrEmptyIntent", err)
	}
}

func TestKitLLMGenerator_RejectsBadLang(t *testing.T) {
	fc := &fakeCompleter{}
	g := create.NewGeneratorFromCompleter(fc, "m")

	_, err := g.Generate(context.Background(), "x", "ruby", nil)
	if err == nil || !strings.Contains(err.Error(), "invalid lang") {
		t.Errorf("err = %v, want invalid lang", err)
	}
}

func TestKitLLMGenerator_TolerantCodeFences(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"name":   "x",
		"events": []string{"PreToolUse"},
		"script": "#!/bin/sh\nexit 0\n",
	})
	wrapped := "```json\n" + string(body) + "\n```\n"
	fc := &fakeCompleter{resp: llm.Response{Content: wrapped}}
	g := create.NewGeneratorFromCompleter(fc, "m")

	res, err := g.Generate(context.Background(), "x", "sh", nil)
	if err != nil {
		t.Fatalf("Generate (code fence): %v", err)
	}
	if res.Name != "x" {
		t.Errorf("Name = %q after fence-strip, want x", res.Name)
	}
}

func TestKitLLMGenerator_ErrorOnMissingScript(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"name":   "x",
		"events": []string{"PreToolUse"},
		// script intentionally omitted.
	})
	fc := &fakeCompleter{resp: llm.Response{Content: string(body)}}
	g := create.NewGeneratorFromCompleter(fc, "m")

	_, err := g.Generate(context.Background(), "x", "sh", nil)
	if err == nil || !strings.Contains(err.Error(), "missing script") {
		t.Errorf("err = %v, want missing script", err)
	}
}

func TestDeriveName_KebabsAndTruncates(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"warn when files >1MB are written", "warn-when-files-1mb-are-written"},
		{"this intent has way too many words to keep", "this-intent-has-way-too-many"},
		{"BLOCK rm -rf!!", "block-rm-rf"},
		{"   ", "hook"},
	}
	for _, c := range cases {
		got := create.DeriveName(c.in)
		if got != c.want {
			t.Errorf("DeriveName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRun_DryRun_NoFilesWritten(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	chdir(t, tmp)

	stub := &stubGenerator{res: &create.Result{
		Name:      "block-rm-rf",
		Script:    "#!/bin/sh\necho '{\"action\":\"BLOCK\"}'\n",
		Events:    []string{"PreToolUse"},
		Rationale: "block destructive rm",
	}}

	var buf bytes.Buffer
	report, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent: "block rm -rf in project root",
		Lang:   "sh",
		DryRun: true,
		Stdout: &buf,
	})
	if err != nil {
		t.Fatalf("Run dry: %v", err)
	}
	if !report.DryRun {
		t.Error("report.DryRun = false")
	}
	if report.ScriptPath != "" {
		t.Errorf("ScriptPath should be empty in dry-run, got %q", report.ScriptPath)
	}
	if report.HooksYAML != "" {
		t.Errorf("HooksYAML should be empty in dry-run, got %q", report.HooksYAML)
	}
	// Verify no .nerv/ was created.
	if _, err := os.Stat(filepath.Join(tmp, ".nerv")); !os.IsNotExist(err) {
		t.Errorf(".nerv/ should not exist after dry-run, err=%v", err)
	}
	// Verify XDG_DATA_HOME also empty.
	if _, err := os.Stat(filepath.Join(tmp, "data", "nerv", "hooks", "block-rm-rf")); !os.IsNotExist(err) {
		t.Errorf("XDG dir should not exist after dry-run, err=%v", err)
	}
	// Verify stdout contains key artefacts.
	out := buf.String()
	for _, want := range []string{
		"dry-run",
		"block-rm-rf",
		"PreToolUse",
		"#!/bin/sh",
		"hooks.yaml entry",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q\nfull:\n%s", want, out)
		}
	}
}

func TestRun_WriteFlow_WritesAndRegisters(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	projDir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, projDir)

	stub := &stubGenerator{res: &create.Result{
		Name:      "audit-shell",
		Script:    "#!/bin/sh\nread env\necho '{\"action\":\"ALLOW\"}'\n",
		Events:    []string{"PreToolUse", "PostToolUse"},
		Rationale: "audit every shell command",
	}}

	report, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent:         "audit-log every shell command",
		Lang:           "sh",
		AllowUntrusted: true,
		LookPath:       func(string) (string, error) { return "", os.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Name != "audit-shell" {
		t.Errorf("Name = %q, want audit-shell", report.Name)
	}
	if report.ScriptPath == "" {
		t.Error("ScriptPath empty after write flow")
	}
	if !strings.HasPrefix(report.ScriptPath, filepath.Join(tmp, "data", "nerv", "hooks", "audit-shell")) {
		t.Errorf("ScriptPath = %q, want under data/nerv/hooks/audit-shell", report.ScriptPath)
	}

	// Handler file lives at $XDG_DATA_HOME/nerv/hooks/<name>/handler.
	if data, err := os.ReadFile(report.ScriptPath); err != nil {
		t.Fatalf("read handler: %v", err)
	} else if !strings.Contains(string(data), "ALLOW") {
		t.Errorf("handler content unexpected: %q", string(data))
	}

	// hooks.yaml exists and registers the hook.
	yamlPath := filepath.Join(projDir, ".nerv", "hooks.yaml")
	yb, err := os.ReadFile(yamlPath)
	if err != nil {
		t.Fatalf("read hooks.yaml: %v", err)
	}
	if !strings.Contains(string(yb), "audit-shell") {
		t.Errorf("hooks.yaml missing entry: %s", yb)
	}
	if !strings.Contains(string(yb), "PreToolUse") {
		t.Errorf("hooks.yaml missing inferred event: %s", yb)
	}

	// Lockfile exists and tracks the install.
	lockPath := filepath.Join(projDir, ".nerv", "hooks.lock")
	lf, err := hookpkg.ReadLockfile(lockPath)
	if err != nil {
		t.Fatalf("read lockfile: %v", err)
	}
	if !lf.Has("audit-shell") {
		t.Errorf("lockfile missing audit-shell")
	}
}

func TestRun_NameOverride_BeatsModelOutput(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	projDir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, projDir)

	stub := &stubGenerator{res: &create.Result{
		Name:   "model-suggested",
		Script: "#!/bin/sh\nexit 0\n",
		Events: []string{"PreToolUse"},
	}}

	report, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent:         "some intent",
		Lang:           "sh",
		NameOverride:   "my-custom-name",
		AllowUntrusted: true,
		LookPath:       func(string) (string, error) { return "", os.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Name != "my-custom-name" {
		t.Errorf("Name = %q, want my-custom-name", report.Name)
	}
}

func TestRun_NameFallback_DerivedWhenModelEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	projDir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, projDir)

	stub := &stubGenerator{res: &create.Result{
		Name:   "", // model returned no name
		Script: "#!/bin/sh\nexit 0\n",
		Events: []string{"PreToolUse"},
	}}

	report, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent:         "audit shell commands always",
		Lang:           "sh",
		AllowUntrusted: true,
		LookPath:       func(string) (string, error) { return "", os.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := create.DeriveName("audit shell commands always")
	if report.Name != want {
		t.Errorf("Name = %q, want derived %q", report.Name, want)
	}
}

func TestRun_RejectsInvalidLang(t *testing.T) {
	stub := &stubGenerator{res: &create.Result{}}
	_, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent: "x",
		Lang:   "ruby",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid lang") {
		t.Errorf("err = %v, want invalid lang", err)
	}
}

func TestRun_RejectsEmptyIntent(t *testing.T) {
	stub := &stubGenerator{res: &create.Result{}}
	_, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent: "",
		Lang:   "sh",
	})
	if !errors.Is(err, create.ErrEmptyIntent) {
		t.Errorf("err = %v, want ErrEmptyIntent", err)
	}
}

func TestRun_PostInstallVerifyFails_RollsBack(t *testing.T) {
	tmp := t.TempDir()
	dataHome := filepath.Join(tmp, "data")
	t.Setenv("XDG_DATA_HOME", dataHome)
	projDir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, projDir)

	stub := &stubGenerator{res: &create.Result{
		Name:   "broken-hook",
		Script: "#!/bin/sh\necho ok\n",
		Events: []string{"PreToolUse"},
	}}

	// Inject a VerifyFn that always fails — Run should remove the hook
	// + return the wrapped error.
	_, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent:         "x",
		Lang:           "sh",
		AllowUntrusted: true,
		LookPath:       func(string) (string, error) { return "", os.ErrNotExist },
		VerifyFn: func(_ string) error {
			return errors.New("simulated lint failure")
		},
	})
	if err == nil {
		t.Fatal("Run: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "simulated lint failure") {
		t.Errorf("err = %v, want simulated lint failure", err)
	}

	// hooks.lock must NOT list broken-hook after rollback.
	lockPath := filepath.Join(projDir, ".nerv", "hooks.lock")
	lf, err := hookpkg.ReadLockfile(lockPath)
	if err != nil {
		t.Fatalf("read lockfile: %v", err)
	}
	if lf.Has("broken-hook") {
		t.Error("lockfile still lists broken-hook after rollback")
	}

	// Handler files must be purged.
	handler := filepath.Join(dataHome, "nerv", "hooks", "broken-hook", "handler")
	if _, err := os.Stat(handler); !os.IsNotExist(err) {
		t.Errorf("handler not purged after rollback: err=%v", err)
	}
}

// TestRun_RejectsUnknownEvents pins the post-Generate canonical-event
// guard (review #5). An LLM that hallucinates "TotallyMadeUpEvent"
// must fail-fast before hooks.yaml is touched — silent
// never-dispatches are an opaque failure mode.
func TestRun_RejectsUnknownEvents(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	projDir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, projDir)

	stub := &stubGenerator{res: &create.Result{
		Name:   "halluci-hook",
		Script: "#!/bin/sh\nexit 0\n",
		Events: []string{"PreToolUse", "TotallyMadeUpEvent"},
	}}

	_, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent: "x",
		Lang:   "sh",
	})
	if err == nil {
		t.Fatal("Run: expected error for unknown event, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "TotallyMadeUpEvent") {
		t.Errorf("err = %v, want mention of TotallyMadeUpEvent", err)
	}
	// PreToolUse is valid — the error should NOT cite it as unknown.
	// (It may still appear elsewhere in the message; we assert the
	// invalid one is in the cited list.)
	if !strings.Contains(msg, "unknown event") {
		t.Errorf("err = %v, want unknown event phrase", err)
	}
	if !strings.Contains(msg, "--events") {
		t.Errorf("err = %v, want --events override hint", err)
	}

	// No side effects: hooks.yaml and the XDG handler must not exist.
	if _, err := os.Stat(filepath.Join(projDir, ".nerv", "hooks.yaml")); !os.IsNotExist(err) {
		t.Errorf("hooks.yaml created despite validation failure: err=%v", err)
	}
	handler := filepath.Join(tmp, "data", "nerv", "hooks", "halluci-hook", "handler")
	if _, err := os.Stat(handler); !os.IsNotExist(err) {
		t.Errorf("handler created despite validation failure: err=%v", err)
	}
}

// TestRun_RejectsAllUnknownEvents covers the all-bad case — the error
// message must list every offender so the operator can fix them in
// one pass.
func TestRun_RejectsAllUnknownEvents(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	chdir(t, tmp)

	stub := &stubGenerator{res: &create.Result{
		Name:   "all-bad",
		Script: "#!/bin/sh\nexit 0\n",
		Events: []string{"FakeEventA", "FakeEventB"},
	}}

	_, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent: "x",
		Lang:   "sh",
		DryRun: true,
	})
	if err == nil {
		t.Fatal("Run: expected error, got nil")
	}
	for _, want := range []string{"FakeEventA", "FakeEventB"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, missing %q", err, want)
		}
	}
}

func TestRun_GeneratorError_Propagates(t *testing.T) {
	stub := &stubGenerator{err: errors.New("transport down")}
	_, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent: "x",
		Lang:   "sh",
	})
	if err == nil || !strings.Contains(err.Error(), "transport down") {
		t.Errorf("err = %v, want transport down propagation", err)
	}
}

// --- review #10: safety gate + linter ---------------------------------

// TestRun_BlocksWriteWithoutAllowUntrusted pins the default safety
// gate. A write-mode Run (no --dry-run, no --allow-untrusted) must
// print the script for audit, then fail with ErrUntrustedHandler
// before any disk write. The error must mention --allow-untrusted so
// the operator knows the escape hatch.
func TestRun_BlocksWriteWithoutAllowUntrusted(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	projDir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, projDir)

	stub := &stubGenerator{res: &create.Result{
		Name:   "blocked-hook",
		Script: "#!/bin/sh\nrm -rf /\n",
		Events: []string{"PreToolUse"},
	}}

	var out bytes.Buffer
	_, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent: "block writes",
		Lang:   "sh",
		Stdout: &out,
	})
	if err == nil {
		t.Fatal("Run: expected ErrUntrustedHandler, got nil")
	}
	if !errors.Is(err, create.ErrUntrustedHandler) {
		t.Errorf("err = %v, want ErrUntrustedHandler", err)
	}
	if !strings.Contains(err.Error(), "--allow-untrusted") {
		t.Errorf("err = %v, want --allow-untrusted hint", err)
	}
	// Script must have been printed for audit.
	body := out.String()
	if !strings.Contains(body, "rm -rf") {
		t.Errorf("audit output missing script body: %s", body)
	}
	if !strings.Contains(body, "blocked-hook") {
		t.Errorf("audit output missing name: %s", body)
	}
	// No disk writes — neither hooks.yaml nor handler exist.
	if _, err := os.Stat(filepath.Join(projDir, ".nerv", "hooks.yaml")); !os.IsNotExist(err) {
		t.Errorf("hooks.yaml created despite safety gate: err=%v", err)
	}
	handler := filepath.Join(tmp, "data", "nerv", "hooks", "blocked-hook", "handler")
	if _, err := os.Stat(handler); !os.IsNotExist(err) {
		t.Errorf("handler created despite safety gate: err=%v", err)
	}
}

// TestRun_LinterFailureBlocksWrite covers the second gate: even with
// --allow-untrusted, a syntactic linter finding aborts the write.
// LookPath returns a fake binary so the test runs the same regardless
// of host install state.
func TestRun_LinterFailureBlocksWrite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	projDir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, projDir)

	stub := &stubGenerator{res: &create.Result{
		Name:   "syntax-bad",
		Script: "#!/bin/sh\nif then fi\n",
		Events: []string{"PreToolUse"},
	}}

	_, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent:         "syntax bad",
		Lang:           "sh",
		AllowUntrusted: true,
		LookPath: func(name string) (string, error) {
			if name == "shellcheck" {
				return "/usr/local/bin/shellcheck", nil
			}
			return "", os.ErrNotExist
		},
		RunLinter: func(name string, args ...string) ([]byte, error) {
			return []byte("syntax error near 'then'"), errors.New("exit status 1")
		},
	})
	if err == nil {
		t.Fatal("Run: expected linter failure, got nil")
	}
	if !strings.Contains(err.Error(), "shellcheck") {
		t.Errorf("err = %v, want shellcheck reference", err)
	}
	if !strings.Contains(err.Error(), "syntax error near") {
		t.Errorf("err = %v, want linter output", err)
	}
	// No disk write.
	handler := filepath.Join(tmp, "data", "nerv", "hooks", "syntax-bad", "handler")
	if _, err := os.Stat(handler); !os.IsNotExist(err) {
		t.Errorf("handler written despite linter failure: err=%v", err)
	}
}

// TestRun_LinterCleanPasses confirms the happy path — linter
// available, exits 0 — proceeds to install.
func TestRun_LinterCleanPasses(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	projDir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, projDir)

	stub := &stubGenerator{res: &create.Result{
		Name:   "lint-clean",
		Script: "#!/bin/sh\nexit 0\n",
		Events: []string{"PreToolUse"},
	}}

	var out bytes.Buffer
	report, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent:         "clean",
		Lang:           "sh",
		AllowUntrusted: true,
		Stdout:         &out,
		LookPath: func(name string) (string, error) {
			if name == "shellcheck" {
				return "/fake/shellcheck", nil
			}
			return "", os.ErrNotExist
		},
		RunLinter: func(string, ...string) ([]byte, error) {
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.ScriptPath == "" {
		t.Error("ScriptPath empty after clean lint")
	}
	if !strings.Contains(out.String(), "shellcheck: clean") {
		t.Errorf("expected shellcheck clean marker, got: %s", out.String())
	}
}

// TestRun_LinterAbsentWarnsAndProceeds confirms graceful degradation
// — no linter binary on PATH means a warning to stdout, but the
// install still goes through. The operator already accepted residual
// risk via --allow-untrusted.
func TestRun_LinterAbsentWarnsAndProceeds(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	projDir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, projDir)

	stub := &stubGenerator{res: &create.Result{
		Name:   "no-linter",
		Script: "#!/bin/sh\nexit 0\n",
		Events: []string{"PreToolUse"},
	}}

	var out bytes.Buffer
	report, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent:         "no-linter",
		Lang:           "sh",
		AllowUntrusted: true,
		Stdout:         &out,
		LookPath:       func(string) (string, error) { return "", os.ErrNotExist },
		RunLinter: func(string, ...string) ([]byte, error) {
			t.Fatal("runLinter should not be called when no linter is on PATH")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.ScriptPath == "" {
		t.Error("ScriptPath empty despite proceed-on-missing-linter")
	}
	if !strings.Contains(out.String(), "no sh linter found") {
		t.Errorf("expected missing-linter warning, got: %s", out.String())
	}
}

// TestRun_PrintsScriptBeforeWrite asserts the audit-print precedes
// the write — even on the happy path, the script body lands on stdout
// so the operator can read what was about to be installed (and what
// did get installed when AllowUntrusted is true).
func TestRun_PrintsScriptBeforeWrite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	projDir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, projDir)

	stub := &stubGenerator{res: &create.Result{
		Name:   "audit-print",
		Script: "#!/bin/sh\necho audit-marker\n",
		Events: []string{"PreToolUse"},
	}}

	var out bytes.Buffer
	_, err := create.Run(context.Background(), stub, create.RunOptions{
		Intent:         "audit print",
		Lang:           "sh",
		AllowUntrusted: true,
		Stdout:         &out,
		LookPath:       func(string) (string, error) { return "", os.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	body := out.String()
	if !strings.Contains(body, "audit-marker") {
		t.Errorf("audit output missing script body: %s", body)
	}
	if !strings.Contains(body, "hooks.yaml entry") {
		t.Errorf("audit output missing yaml entry section: %s", body)
	}
}

// --- helpers ---

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
