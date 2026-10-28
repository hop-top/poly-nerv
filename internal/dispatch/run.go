//go:build !windows

package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"hop.top/kit/go/runtime/bus"
	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/hookpkg/builtins"
	"hop.top/nerv/internal/merger"
)

// Runner executes hook handlers.
type Runner struct {
	slowThreshold time.Duration // warn if single hook > this
	bus           bus.Bus       // optional event bus for hook handlers; nil = no publishes
}

// NewRunner creates a handler runner with a 100ms slow threshold and no bus.
func NewRunner() *Runner {
	return &Runner{slowThreshold: 100 * time.Millisecond}
}

// NewRunnerWithBus creates a handler runner wired to the given bus.
// The dispatcher publishes crtx emissions through this reference
// after every dispatch. Pass nil to skip bus wiring.
func NewRunnerWithBus(b bus.Bus) *Runner {
	return &Runner{slowThreshold: 100 * time.Millisecond, bus: b}
}

// Bus returns the bus instance the runner was created with, or nil if
// none was wired. Callers should nil-check before publishing.
func (r *Runner) Bus() bus.Bus {
	return r.bus
}

// exitCodeToAction maps handler exit codes to actions per
// spec/handler-contract.md.
func exitCodeToAction(code int) (merger.Action, bool) {
	switch code {
	case 0:
		return merger.Allow, true
	case 1:
		return merger.Warn, true
	case 2:
		return merger.Block, true
	case 3:
		return merger.Rewrite, true
	default:
		return "", false // >=4 → error, fail-open
	}
}

// stdoutDecision is the JSON structure a handler may write to stdout.
type stdoutDecision struct {
	Action  string         `json:"action"`
	Message string         `json:"message,omitempty"`
	Rewrite any            `json:"rewrite,omitempty"`
	Meta    map[string]any `json:"metadata,omitempty"`
}

// resolveTimeout returns the effective timeout for a hook.
const (
	minTimeoutSec = 1
	maxTimeoutSec = 300
	defTimeoutSec = 10
)

func resolveTimeout(h config.HookEntry, defaults config.HookDefaults) time.Duration {
	sec := defTimeoutSec
	if defaults.Timeout > 0 {
		sec = defaults.Timeout
	}
	if h.Timeout != nil && *h.Timeout > 0 {
		sec = *h.Timeout
	}
	if sec < minTimeoutSec {
		sec = minTimeoutSec
	}
	if sec > maxTimeoutSec {
		sec = maxTimeoutSec
	}
	return time.Duration(sec) * time.Second
}

// RunHooks executes matched hooks and returns their decisions.
//
// Sequential by default (hooks.yaml order). Per hook:
//  1. Check Source: if "builtin", run in-process via runBuiltin
//  2. Else: fork process: exec hook.Run with envelope on stdin
//  3. Set env vars: NERV_EVENT, NERV_CLI, NERV_SESSION_ID, NERV_CWD, NERV_HOOK_NAME
//  4. Apply timeout (hook.Timeout or default from config)
//  5. Read stdout as Decision JSON
//  6. Collect exit code → map to action per handler-contract.md
//  7. On mismatch between stdout action and exit code: exit code wins, log warning
func (r *Runner) RunHooks(
	hooks []config.HookEntry,
	env *envelope.Envelope,
	cfg *config.HooksConfig,
) []merger.Decision {
	if len(hooks) == 0 {
		return nil
	}

	decisions := make([]merger.Decision, 0, len(hooks))
	for _, h := range hooks {
		var d merger.Decision
		if h.Source == "builtin" {
			d = r.runBuiltin(h, env, cfg)
		} else {
			d = r.runOne(h, env, cfg.Defaults)
		}
		decisions = append(decisions, d)
	}
	return decisions
}

// runBuiltin dispatches to the in-process BuiltinDef.Func.
// Fail-open on: disabled, unknown name, panic, error.
func (r *Runner) runBuiltin(
	h config.HookEntry,
	env *envelope.Envelope,
	cfg *config.HooksConfig,
) merger.Decision {
	name := strings.TrimPrefix(h.Name, "builtin:")
	def, ok := builtins.ByName(name)
	if !ok {
		log.Printf("dispatch: unknown builtin %q (fail-open)", name)
		return allowDecision("fail-open: unknown builtin")
	}

	// Check if built-in is enabled.
	if builtinCfg, ok := cfg.Builtin[name]; ok {
		if builtinCfg.Enabled != nil && !*builtinCfg.Enabled {
			return merger.Decision{Action: merger.Allow}
		}
	}

	// Resolve runtime configuration for the built-in.
	runtimeCfg := builtins.BuiltinRuntimeCfg{
		StateDir: filepath.Join(os.Getenv("HOME"), ".nerv", "state", name),
	}

	if builtinCfg, ok := cfg.Builtin[name]; ok {
		runtimeCfg.UserCfg = builtinCfg.Extra
	}

	timeout := resolveTimeout(h, cfg.Defaults)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	type result struct {
		d   merger.Decision
		err error
	}
	done := make(chan result, 1)

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				done <- result{err: fmt.Errorf("panic: %v", rec)}
			}
		}()
		d, err := def.Func(env, runtimeCfg)
		done <- result{d: d, err: err}
	}()

	select {
	case <-ctx.Done():
		log.Printf("dispatch: builtin %q timed out after %v", name, timeout)
		return allowDecision(fmt.Sprintf("fail-open: timeout after %v", timeout))
	case res := <-done:
		if res.err != nil {
			log.Printf("dispatch: builtin %q error (fail-open): %v", name, res.err)
			return allowDecision(fmt.Sprintf("fail-open: builtin error: %v", res.err))
		}
		return res.d
	}
}

// runOne executes a single hook handler and returns its decision.
func (r *Runner) runOne(
	h config.HookEntry,
	env *envelope.Envelope,
	defaults config.HookDefaults,
) merger.Decision {
	timeout := resolveTimeout(h, defaults)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", h.Run)

	// Kill entire process group on context cancellation so child
	// processes (e.g. sleep) don't outlive the parent shell.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			// Graceful: SIGTERM first, SIGKILL after WaitDelay.
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
		return nil
	}
	cmd.WaitDelay = 5 * time.Second // SIGKILL after 5s if SIGTERM ignored

	// Encode envelope to stdin.
	var stdinBuf bytes.Buffer
	if err := envelope.Encode(&stdinBuf, env); err != nil {
		log.Printf("dispatch: hook %q: stdin encode error: %v", h.Name, err)
		return allowDecision("fail-open: stdin encode error")
	}
	cmd.Stdin = &stdinBuf

	// Capture stdout and stderr.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set NERV env vars.
	cmd.Env = append(os.Environ(),
		"NERV_EVENT="+env.Event,
		"NERV_CLI="+env.CLI.Name,
		"NERV_SESSION_ID="+env.Session.ID,
		"NERV_CWD="+env.Session.CWD,
		"NERV_HOOK_NAME="+h.Name,
	)

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	if elapsed > r.slowThreshold {
		log.Printf("dispatch: hook %q slow: %v", h.Name, elapsed)
	}

	// Handle context deadline exceeded (timeout).
	if ctx.Err() == context.DeadlineExceeded {
		log.Printf("dispatch: hook %q timed out after %v", h.Name, timeout)
		return allowDecision(fmt.Sprintf("fail-open: timeout after %v", timeout))
	}

	// Extract exit code.
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if ok := isExitError(err, &exitErr); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Signal kill, SIGPIPE, or other exec failure.
			log.Printf("dispatch: hook %q exec error: %v", h.Name, err)
			return allowDecision(fmt.Sprintf("fail-open: exec error: %v", err))
		}
	}

	// Exit >=4 → fail-open.
	action, known := exitCodeToAction(exitCode)
	if !known {
		stderrMsg := strings.TrimSpace(stderr.String())
		log.Printf("dispatch: hook %q exit %d (fail-open): %s",
			h.Name, exitCode, stderrMsg)
		return allowDecision(fmt.Sprintf("fail-open: exit code %d", exitCode))
	}

	// Parse stdout JSON (empty = allow per handler-contract.md).
	raw := bytes.TrimSpace(stdout.Bytes())
	if len(raw) == 0 {
		return merger.Decision{Action: action}
	}

	var out stdoutDecision
	if err := json.Unmarshal(raw, &out); err != nil {
		log.Printf("dispatch: hook %q: malformed stdout JSON: %v", h.Name, err)
		return allowDecision("fail-open: malformed stdout JSON")
	}

	// Build decision from stdout.
	decision := merger.Decision{
		Message:  out.Message,
		Rewrite:  out.Rewrite,
		Metadata: out.Meta,
	}

	// Resolve action: if stdout declares an action, check against exit code.
	if out.Action != "" {
		stdoutAction := merger.Action(out.Action)
		if stdoutAction != action {
			log.Printf("dispatch: hook %q: stdout action %q != exit code %d (%s); exit code wins",
				h.Name, out.Action, exitCode, action)
		}
	}
	decision.Action = action // exit code always wins

	return decision
}

// allowDecision returns an allow decision with a message.
func allowDecision(msg string) merger.Decision {
	return merger.Decision{
		Action:  merger.Allow,
		Message: msg,
	}
}

// isExitError checks if err is an *exec.ExitError using errors.As.
func isExitError(err error, target **exec.ExitError) bool {
	if err == nil {
		return false
	}
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	// Check wrapped errors via Unwrap.
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return isExitError(u.Unwrap(), target)
	}
	return false
}
