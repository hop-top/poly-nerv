package create

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/hookpkg"
)

// RunOptions captures the verb-side knobs that Run accepts. The verb
// translates its cobra flags into this struct.
type RunOptions struct {
	Intent       string
	Lang         string
	Events       []string // override (verbatim), empty = let the LLM infer
	NameOverride string   // empty = derive from intent / model output
	DryRun       bool
	Stdout       io.Writer

	// AllowUntrusted gates the disk-write step on LLM-generated
	// handlers. Default false: Run prints the generated script + yaml
	// entry to Stdout and returns ErrUntrustedHandler so the operator
	// can audit before re-running with --allow-untrusted. Set true to
	// proceed with the write (still subject to linter gate).
	AllowUntrusted bool

	// VerifyFn, if non-nil, replaces the default post-install handler
	// check. Used by tests to drive the rollback path deterministically.
	// Production callers leave this nil — Run falls back to verifyHandler.
	VerifyFn func(handlerPath string) error

	// LookPath, if non-nil, overrides exec.LookPath for linter
	// discovery (shellcheck, ruff, python3). Tests inject a custom
	// resolver to exercise the gate deterministically across machines
	// that may or may not have the linters installed.
	LookPath func(name string) (string, error)

	// RunLinter, if non-nil, overrides the actual linter subprocess
	// invocation. Tests inject this to simulate pass/fail without
	// requiring the real binary on PATH. Signature mirrors
	// exec.Command(name, args...).CombinedOutput.
	RunLinter func(name string, args ...string) ([]byte, error)
}

// ErrUntrustedHandler is returned by Run when the default safety gate
// blocks a write — i.e. AllowUntrusted is false and DryRun is false.
// The operator must re-run with --allow-untrusted after auditing the
// printed script. Wrapped via fmt.Errorf so callers can detect it via
// errors.Is.
var ErrUntrustedHandler = errors.New(
	"create: refusing to write LLM-generated handler without --allow-untrusted",
)

// RunReport summarises a successful Run for the verb to print/inspect.
type RunReport struct {
	Name       string
	Lang       string
	Events     []string
	ScriptPath string // empty in --dry-run
	HooksYAML  string // empty in --dry-run
	YAMLEntry  string // hooks.yaml entry that would be / was written
	Rationale  string
	DryRun     bool
	Generation *Result // raw generator output (for diagnostics)
}

// Run drives the full create flow: generate → name resolution → write
// script → register via hookpkg.Install. Used by cmd/nerv/cmd_create.go
// and exercised end-to-end by the package's tests.
//
// On --dry-run, Run prints the generated artifacts to opts.Stdout and
// does NOT touch disk.
func Run(ctx context.Context, gen Generator, opts RunOptions) (*RunReport, error) {
	if err := ValidateLang(opts.Lang); err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.Intent) == "" {
		return nil, ErrEmptyIntent
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}

	res, err := gen.Generate(ctx, opts.Intent, opts.Lang, opts.Events)
	if err != nil {
		return nil, err
	}

	// Fail-fast on event names the daemon won't dispatch. The LLM
	// occasionally hallucinates events (typos, deprecated names,
	// neighbouring-product spellings); writing them into hooks.yaml
	// makes the hook silently never fire. Users hit a "where's my
	// hook?" debugging session with no signal from the verb. The
	// canonical catalog lives in internal/events; we surface every
	// unknown name in one error so the operator can re-prompt with an
	// explicit --events override.
	if bad := unknownEvents(res.Events); len(bad) > 0 {
		return nil, fmt.Errorf(
			"create: generator returned unknown event(s) %v "+
				"(known catalog in internal/events); "+
				"re-run with --events <csv> to override",
			bad,
		)
	}

	// Name resolution: --name > model output > derived from intent.
	name := opts.NameOverride
	if name == "" {
		name = res.Name
	}
	if name == "" {
		name = DeriveName(opts.Intent)
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil, fmt.Errorf("create: empty hook name after resolution")
	}

	// Build the hooks.yaml entry representation. Install writes the real
	// entry; we surface a YAML rendering for dry-run + reporting.
	yamlEntry, err := renderYAMLEntry(name, res.Events)
	if err != nil {
		return nil, fmt.Errorf("create: render yaml: %w", err)
	}

	report := &RunReport{
		Name:       name,
		Lang:       opts.Lang,
		Events:     res.Events,
		YAMLEntry:  yamlEntry,
		Rationale:  res.Rationale,
		DryRun:     opts.DryRun,
		Generation: res,
	}

	if opts.DryRun {
		printDryRun(opts.Stdout, name, opts.Lang, res, yamlEntry)
		return report, nil
	}

	// Make the generated artefacts visible BEFORE any disk write — the
	// operator should be able to scroll back and read what nerv was
	// about to install. This mirrors the dry-run output so a re-run
	// with --allow-untrusted doesn't change what the user sees.
	printDryRun(opts.Stdout, name, opts.Lang, res, yamlEntry)

	// Safety gate: refuse to write LLM-generated executables to the
	// user's hook directory unless the operator explicitly opts in.
	// A prompt-injection in the natural-language intent could ship a
	// malicious handler; --allow-untrusted is the deliberate "I read
	// the script above" acknowledgement. Daemon dispatch runs handlers
	// in-process as the user, so this is a privilege-escalation
	// surface, not just a typo guard.
	if !opts.AllowUntrusted {
		return nil, fmt.Errorf(
			"%w (audit the script above; re-run with --allow-untrusted "+
				"to commit)",
			ErrUntrustedHandler,
		)
	}

	// Pre-write linter gate. We run a syntactic check on the
	// generated script before it lands in $XDG_DATA_HOME — a syntax
	// error here would only surface at first dispatch, by which time
	// the file is installed + hooks.yaml is patched. Linters are
	// best-effort: when absent we surface a warning and continue. The
	// operator who passed --allow-untrusted has already accepted the
	// remaining residual risk.
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	runLinter := opts.RunLinter
	if runLinter == nil {
		runLinter = func(name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		}
	}
	if err := lintGenerated(opts.Stdout, opts.Lang, res.Script, lookPath, runLinter); err != nil {
		return nil, fmt.Errorf("create: lint generated handler: %w", err)
	}

	// Write the script to a temp file, then let hookpkg.Install copy it
	// into $XDG_DATA_HOME/nerv/hooks/<name>/handler and register it.
	tmp, err := os.CreateTemp("", "nerv-create-*."+FileExt(opts.Lang))
	if err != nil {
		return nil, fmt.Errorf("create: tempfile: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(res.Script); err != nil {
		_ = tmp.Close()        // best-effort cleanup; the write error is returned
		_ = os.Remove(tmpPath) // best-effort cleanup
		return nil, fmt.Errorf("create: write tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup
		return nil, fmt.Errorf("create: close tempfile: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup
		return nil, fmt.Errorf("create: chmod tempfile: %w", err)
	}

	defer func() { _ = os.Remove(tmpPath) }() // Install copied it; temp cleanup is best-effort

	if err := hookpkg.Install(name, tmpPath); err != nil {
		return nil, fmt.Errorf("create: install %q: %w", name, err)
	}

	// Patch the hooks.yaml entry to record the inferred event list. The
	// default Install path sets On="tool_call"; we want the model's
	// inferred events to land in the registered entry too.
	dirs, err := hookpkg.ResolveDirs(name)
	if err != nil {
		return nil, fmt.Errorf("create: resolve dirs: %w", err)
	}
	if len(res.Events) > 0 {
		if err := patchHookEvents(dirs.HooksYAML, name, res.Events); err != nil {
			return nil, fmt.Errorf("create: patch hooks.yaml: %w", err)
		}
	}

	// Resolve the installed handler path for the report.
	report.ScriptPath = filepath.Join(dirs.DataDir, "handler")
	report.HooksYAML = dirs.HooksYAML

	// Post-install verification: handler must be on disk + executable.
	// On failure, roll back the install so the project state stays
	// clean. This mirrors the slash-command spec's lint step without
	// shelling out — `nerv lint` and `nerv test` remain available as
	// follow-up commands. Tests inject a custom VerifyFn to exercise
	// the rollback path deterministically.
	verify := opts.VerifyFn
	if verify == nil {
		verify = verifyHandler
	}
	if err := verify(report.ScriptPath); err != nil {
		_ = hookpkg.Remove(name, true)
		return nil, fmt.Errorf("create: post-install verify: %w", err)
	}

	return report, nil
}

// lintGenerated runs an available linter against script and returns
// an error on findings. Linter discovery is lang-keyed:
//
//	sh → shellcheck -S warning
//	py → ruff check, else python3 -m py_compile
//
// When no linter is on PATH (looked up via lookPath), the function
// writes a one-line "skipping" notice to w and returns nil — the user
// already accepted residual risk via --allow-untrusted. Errors from
// the subprocess itself (looked up but failed to spawn) propagate.
func lintGenerated(
	w io.Writer,
	lang string,
	script string,
	lookPath func(string) (string, error),
	runLinter func(string, ...string) ([]byte, error),
) error {
	tmp, err := os.CreateTemp("", "nerv-lint-*."+FileExt(lang))
	if err != nil {
		return fmt.Errorf("tempfile: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // temp cleanup is best-effort

	if _, err := tmp.WriteString(script); err != nil {
		_ = tmp.Close() // the write error is returned
		return fmt.Errorf("write tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close tempfile: %w", err)
	}

	type linter struct {
		bin  string
		args []string
	}
	var candidates []linter
	switch lang {
	case "sh":
		candidates = []linter{
			{bin: "shellcheck", args: []string{"-S", "warning", tmpPath}},
		}
	case "py":
		// Prefer ruff (richer findings); fall back to py_compile.
		candidates = []linter{
			{bin: "ruff", args: []string{"check", tmpPath}},
			{bin: "python3", args: []string{"-m", "py_compile", tmpPath}},
		}
	default:
		// Unknown lang — ValidateLang would have caught this. No-op.
		return nil
	}

	for _, c := range candidates {
		if _, err := lookPath(c.bin); err != nil {
			continue
		}
		out, err := runLinter(c.bin, c.args...)
		if err != nil {
			return fmt.Errorf("%s reported issues:\n%s", c.bin, strings.TrimSpace(string(out)))
		}
		_, _ = fmt.Fprintf(w, "# %s: clean\n", c.bin)
		return nil
	}

	_, _ = fmt.Fprintf(w,
		"# warning: no %s linter found on PATH; "+
			"install shellcheck (sh) or ruff/python3 (py) for syntactic validation\n",
		lang,
	)
	return nil
}

// unknownEvents filters evts down to entries the canonical catalog
// rejects. Used post-Generate to fail-fast before writing hooks.yaml.
// Empty input returns empty (vacuously valid). The result preserves
// input order so the error message reads predictably.
func unknownEvents(evts []string) []string {
	var bad []string
	for _, e := range evts {
		if !events.IsValid(events.Event(e)) {
			bad = append(bad, e)
		}
	}
	return bad
}

// verifyHandler confirms the generated handler exists and is executable.
// Kept narrow on purpose — the broader `nerv lint` and `nerv test`
// surfaces validate event-name correctness, YAML schema, and runtime
// behaviour. Here we only catch install-time corruption.
func verifyHandler(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat handler %s: %w", path, err)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("handler %s is not executable", path)
	}
	return nil
}

// renderYAMLEntry produces a YAML fragment representing the single
// hooks: entry the create flow would emit. The shape mirrors
// config.HookEntry so downstream merging/linting accepts it.
func renderYAMLEntry(name string, events []string) (string, error) {
	var on any = "tool_call"
	if len(events) == 1 {
		on = events[0]
	} else if len(events) > 1 {
		on = events
	}
	entry := []config.HookEntry{
		{Name: name, On: on, Run: ".nerv/hooks/" + name},
	}
	data, err := yaml.Marshal(map[string]any{"hooks": entry})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func printDryRun(w io.Writer, name, lang string, res *Result, yamlEntry string) {
	var b strings.Builder
	fmt.Fprintf(&b, "# nerv create — dry-run\n")
	fmt.Fprintf(&b, "# name:     %s\n", name)
	fmt.Fprintf(&b, "# lang:     %s\n", lang)
	fmt.Fprintf(&b, "# events:   %s\n", strings.Join(res.Events, ", "))
	if res.Rationale != "" {
		fmt.Fprintf(&b, "# why:      %s\n", res.Rationale)
	}
	b.WriteString("\n## hooks.yaml entry\n")
	b.WriteString(yamlEntry + "\n")
	b.WriteString("## handler script\n")
	b.WriteString(res.Script + "\n")
	// Dry-run preview on the CLI's output stream; a failed write has no recovery.
	_, _ = io.WriteString(w, b.String())
}

// patchHookEvents rewrites the `on:` field of the named hook entry in
// hooks.yaml to the supplied event list. The default Install path
// hardcodes `on: tool_call`; this brings the registered entry in line
// with the inferred subscription.
func patchHookEvents(yamlPath, name string, events []string) error {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return err
	}
	var cfg config.HooksConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	patched := false
	for i, h := range cfg.Hooks {
		if h.Name != name {
			continue
		}
		if len(events) == 1 {
			cfg.Hooks[i].On = events[0]
		} else {
			cfg.Hooks[i].On = events
		}
		patched = true
		break
	}
	if !patched {
		return fmt.Errorf("hook %q not found in %s", name, yamlPath)
	}
	out, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(yamlPath, out, 0o644)
}
