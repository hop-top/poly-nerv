//go:build !windows

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/kit/go/core/xdg"
	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/eventlog"
)

// CheckStatus represents the health state of a single check.
type CheckStatus string

const (
	StatusOK   CheckStatus = "OK"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
	StatusSkip CheckStatus = "SKIP"
)

// Check is a single doctor check result.
type Check struct {
	Name    string      `table:"Check"   json:"name"    yaml:"name"`
	Status  CheckStatus `table:"Status"  json:"status"  yaml:"status"`
	Message string      `table:"Message" json:"message" yaml:"message"`
	Detail  string      `table:"-"       json:"detail,omitempty"  yaml:"detail,omitempty"`
}

// adapterCLIs maps adapter names to their expected binary names.
var adapterCLIs = map[string]string{
	"claude":   "claude",
	"codex":    "codex",
	"gemini":   "gemini",
	"opencode": "opencode",
}

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check nerv installation health",
		Long: `Run a suite of read-only health probes against the nerv
installation and report each as OK / WARN / FAIL / SKIP.

Covers detected AI-CLI adapters (claude, codex, gemini, opencode),
hooks.yaml loadability + per-handler executability, event-log
directory writability + recent-entry presence, and the nervd unix
socket. Output honors --format (table, json, yaml) so it can feed
CI gates without further parsing.`,
		RunE: runDoctor,
		Annotations: map[string]string{
			"kit/side-effect":    "read",
			"kit/idempotent":     "yes",
			"kit/top-level-verb": "true",
		},
	}
}

func runDoctor(_ *cobra.Command, _ []string) error {
	var checks []Check

	// 1. Per-adapter checks
	for name, bin := range adapterCLIs {
		checks = append(checks, checkAdapterCLI(name, bin))
	}

	// 2. Hooks config
	checks = append(checks, checkHooksConfig()...)

	// 3. Event log health
	checks = append(checks, checkEventLog())

	// 4. Daemon reachability
	checks = append(checks, checkDaemon())

	format := viper.GetString("format")
	if format == "" {
		format = output.Table
	}

	return output.Render(os.Stdout, format, checks)
}

// checkAdapterCLI verifies a single adapter CLI binary is installed.
func checkAdapterCLI(adapter, binary string) Check {
	name := fmt.Sprintf("adapter/%s", adapter)

	path, err := exec.LookPath(binary)
	if err != nil {
		return Check{
			Name:    name,
			Status:  StatusSkip,
			Message: fmt.Sprintf("%s binary not found on PATH", binary),
		}
	}

	// Try to get version via --version flag.
	cmd := exec.Command(path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return Check{
			Name:    name,
			Status:  StatusWarn,
			Message: fmt.Sprintf("found at %s but --version failed", path),
			Detail:  err.Error(),
		}
	}

	ver := strings.TrimSpace(string(out))
	if len(ver) > 80 {
		ver = ver[:80]
	}
	return Check{
		Name:    name,
		Status:  StatusOK,
		Message: ver,
		Detail:  path,
	}
}

// checkHooksConfig loads and validates hooks.yaml layers.
func checkHooksConfig() []Check {
	var checks []Check

	cfg, err := config.Load()
	if err != nil {
		checks = append(checks, Check{
			Name:    "hooks.yaml",
			Status:  StatusFail,
			Message: "failed to load hooks.yaml",
			Detail:  err.Error(),
		})
		return checks
	}

	if len(cfg.Hooks) == 0 {
		checks = append(checks, Check{
			Name:    "hooks.yaml",
			Status:  StatusWarn,
			Message: "loaded but no hooks defined",
		})
		return checks
	}

	checks = append(checks, Check{
		Name:    "hooks.yaml",
		Status:  StatusOK,
		Message: fmt.Sprintf("%d hook(s), version=%s", len(cfg.Hooks), cfg.Version),
	})

	// Validate each hook handler exists and is executable.
	for _, h := range cfg.Hooks {
		c := checkHookHandler(h)
		checks = append(checks, c)
	}

	return checks
}

// checkHookHandler verifies a single hook's run target exists.
func checkHookHandler(h config.HookEntry) Check {
	name := fmt.Sprintf("hook/%s", h.Name)

	if h.Run == "" {
		return Check{
			Name:    name,
			Status:  StatusFail,
			Message: "no 'run' command specified",
		}
	}

	// Extract the first word as the binary to check.
	parts := strings.Fields(h.Run)
	bin := parts[0]

	// Resolve relative to PATH or absolute.
	resolved, err := exec.LookPath(bin)
	if err != nil {
		return Check{
			Name:    name,
			Status:  StatusWarn,
			Message: fmt.Sprintf("handler binary %q not found", bin),
			Detail:  h.Run,
		}
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return Check{
			Name:    name,
			Status:  StatusWarn,
			Message: fmt.Sprintf("cannot stat %s", resolved),
			Detail:  err.Error(),
		}
	}

	if info.Mode()&0o111 == 0 {
		return Check{
			Name:    name,
			Status:  StatusFail,
			Message: fmt.Sprintf("%s is not executable", resolved),
		}
	}

	return Check{
		Name:    name,
		Status:  StatusOK,
		Message: fmt.Sprintf("executable at %s", resolved),
	}
}

// checkEventLog checks event log directory health.
func checkEventLog() Check {
	logger := &eventlog.JSONLLogger{}
	dir := eventLogDir()

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Check{
				Name:    "eventlog",
				Status:  StatusWarn,
				Message: "log directory does not exist yet",
				Detail:  dir,
			}
		}
		return Check{
			Name:    "eventlog",
			Status:  StatusFail,
			Message: fmt.Sprintf("cannot stat log dir: %v", err),
			Detail:  dir,
		}
	}

	if !info.IsDir() {
		return Check{
			Name:    "eventlog",
			Status:  StatusFail,
			Message: "log path exists but is not a directory",
			Detail:  dir,
		}
	}

	// Check writability by attempting to create a temp file.
	tmp := filepath.Join(dir, ".nerv-doctor-probe")
	if err := os.WriteFile(tmp, []byte("probe"), 0o600); err != nil {
		return Check{
			Name:    "eventlog",
			Status:  StatusFail,
			Message: "log directory not writable",
			Detail:  err.Error(),
		}
	}
	_ = os.Remove(tmp) // probe file; writability is already proven

	// Query recent entries.
	entries, err := logger.Query(eventlog.Filter{
		Since: time.Now().Add(-24 * time.Hour),
		Limit: 1,
	})
	if err != nil {
		return Check{
			Name:    "eventlog",
			Status:  StatusWarn,
			Message: "writable but query failed",
			Detail:  err.Error(),
		}
	}

	if len(entries) == 0 {
		return Check{
			Name:    "eventlog",
			Status:  StatusOK,
			Message: "writable, no entries in last 24h",
			Detail:  dir,
		}
	}

	return Check{
		Name:    "eventlog",
		Status:  StatusOK,
		Message: "writable, recent entries found",
		Detail:  dir,
	}
}

// checkDaemon checks whether the nervd daemon is reachable.
func checkDaemon() Check {
	_, sock := daemonPaths()

	if _, err := os.Stat(sock); err != nil {
		return Check{
			Name:    "daemon",
			Status:  StatusSkip,
			Message: "socket not found (daemon mode not active)",
			Detail:  sock,
		}
	}

	conn, err := net.DialTimeout("unix", sock, 500*time.Millisecond)
	if err != nil {
		return Check{
			Name:    "daemon",
			Status:  StatusFail,
			Message: "socket exists but connection failed",
			Detail:  err.Error(),
		}
	}
	_ = conn.Close() // probe connection; only the dial result matters

	return Check{
		Name:    "daemon",
		Status:  StatusOK,
		Message: "reachable via unix socket",
		Detail:  sock,
	}
}

// eventLogDir returns the event log directory path via kit/xdg: the
// internal events/ subtree of the state root, segregated from the
// sessions/ subtree that crtx consumers scan. Mirrors the JSONLLogger
// default in internal/eventlog so doctor inspects the same directory
// the daemon writes to.
func eventLogDir() string {
	dir, err := xdg.StateDir("nerv")
	if err != nil {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "state", "nerv", "events")
	}
	return filepath.Join(dir, "events")
}
