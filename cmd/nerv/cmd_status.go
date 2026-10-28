//go:build !windows

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/nerv/internal/config"
)

// statusRow is a key-value pair for the status summary.
type statusRow struct {
	Key   string `table:"Property" json:"key"   yaml:"key"`
	Value string `table:"Value"    json:"value" yaml:"value"`
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show nerv daemon and adapter status",
		Long: `Render a compact one-screen summary of the current nerv
installation: binary version, which AI-CLI adapters are detected on
PATH, the loaded hooks.yaml (hook count + active event names), the
daemon's running state (pidfile + socket), and the on-disk event-log
directory.

Read-only — never mutates config, pidfile, or socket. Honors the
global --format flag (table, json, yaml) for downstream tooling.`,
		RunE: runStatus,
		Annotations: map[string]string{
			"kit/side-effect":    "read",
			"kit/idempotent":     "yes",
			"kit/top-level-verb": "true",
		},
	}
}

func runStatus(_ *cobra.Command, _ []string) error {
	var rows []statusRow

	// nerv version.
	rows = append(rows, statusRow{Key: "nerv version", Value: version})

	// Installed adapters.
	installedAdapters := []string{}
	for name, bin := range adapterCLIs {
		if _, err := lookPathSafe(bin); err == nil {
			installedAdapters = append(installedAdapters, name)
		}
	}
	if len(installedAdapters) == 0 {
		rows = append(rows, statusRow{
			Key: "adapters", Value: "none detected",
		})
	} else {
		for i, a := range installedAdapters {
			key := "adapters"
			if i > 0 {
				key = ""
			}
			rows = append(rows, statusRow{Key: key, Value: a})
		}
	}

	// Hooks config.
	cfg, err := config.Load()
	if err != nil {
		rows = append(rows, statusRow{
			Key: "hooks", Value: fmt.Sprintf("error: %v", err),
		})
	} else {
		rows = append(rows, statusRow{
			Key:   "hooks",
			Value: fmt.Sprintf("%d defined (version: %s)", len(cfg.Hooks), cfg.Version),
		})

		// Active events.
		eventSet := make(map[string]bool)
		for _, h := range cfg.Hooks {
			for _, ev := range h.ResolveOn() {
				eventSet[ev] = true
			}
		}
		rows = append(rows, statusRow{
			Key:   "active events",
			Value: fmt.Sprintf("%d unique", len(eventSet)),
		})
	}

	// Daemon status.
	pidfile, sock := daemonPaths()
	pid, err := readPID(pidfile)
	if err != nil {
		rows = append(rows, statusRow{Key: "daemon", Value: "stopped"})
	} else if !processAlive(pid) {
		rows = append(rows, statusRow{
			Key:   "daemon",
			Value: fmt.Sprintf("stopped (stale pid: %d)", pid),
		})
	} else {
		rows = append(rows, statusRow{
			Key:   "daemon",
			Value: fmt.Sprintf("running (pid: %d, socket: %s)", pid, sock),
		})
	}

	// Event log directory.
	dir := eventLogDir()
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		rows = append(rows, statusRow{Key: "event log", Value: dir})
	} else {
		rows = append(rows, statusRow{Key: "event log", Value: "not created yet"})
	}

	format := viper.GetString("format")
	if format == "" {
		format = output.Table
	}

	return output.Render(os.Stdout, format, rows)
}

// lookPathSafe is a simple PATH lookup without exec.
func lookPathSafe(bin string) (string, error) {
	paths := os.Getenv("PATH")
	for _, dir := range splitPath(paths) {
		full := dir + "/" + bin
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			return full, nil
		}
	}
	return "", fmt.Errorf("not found: %s", bin)
}

func splitPath(p string) []string {
	if p == "" {
		return nil
	}
	return splitOn(p, ':')
}

func splitOn(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
