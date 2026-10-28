package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"

	"hop.top/nerv/adapters"
)

// cliStatus represents the installation state of a single AI CLI.
type cliStatus struct {
	CLI         string `table:"CLI"          json:"cli"          yaml:"cli"`
	Installed   string `table:"Installed"    json:"installed"    yaml:"installed"`
	Version     string `table:"Version"      json:"version"      yaml:"version"`
	NervAdapter string `table:"Nerv Adapter" json:"nerv_adapter" yaml:"nerv_adapter"`
	Drift       string `table:"Drift"        json:"drift"        yaml:"drift"`
	LastInstall string `table:"Last Install" json:"last_install" yaml:"last_install"`
}

// knownCLIs is the canonical list of AI CLIs that nerv supports.
var knownCLIs = []string{
	"claude",
	"codex",
	"gemini",
	"aider",
	"goose",
	"continue",
	"cline",
	"roo",
	"kilo",
	"amp",
	"opencode",
	"openhands",
	"vibe",
	"aide",
	"zed",
}

func installStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show adapter installation state for known AI CLIs",
		Long: `Enumerate the AI CLIs that nerv knows how to adapt (claude,
codex, gemini, aider, goose, continue, cline, roo, kilo, amp,
opencode, openhands, vibe, aide, zed) and report a row per CLI with
installation state, detected version, nerv adapter status, drift vs
the original install, and the last install timestamp.

Read-only; honors --format (table, json, yaml). Distinct from the
root 'nerv status' which summarises the running install instead.`,
		RunE: runInstallStatus,
		Annotations: map[string]string{
			"kit/side-effect": "read",
			"kit/idempotent":  "yes",
		},
	}
	return cmd
}

func runInstallStatus(_ *cobra.Command, _ []string) error {
	rows := make([]cliStatus, 0, len(knownCLIs))

	for _, name := range knownCLIs {
		row := buildCLIStatus(name)
		rows = append(rows, row)
	}

	format := viper.GetString("format")
	if format == "" {
		format = output.Table
	}

	if format == output.Table && len(rows) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No known CLIs configured.")
		return nil
	}

	return output.Render(os.Stdout, format, rows)
}

// buildCLIStatus probes a single CLI and returns its status row.
//
// Detection uses the adapter registry: an adapter's Detect() reports
// binary presence + version, the install journal reports adapter
// state + last install, and DetectDrift() compares the current config
// hash against the recorded post-install hash.
func buildCLIStatus(name string) cliStatus {
	dash := "\u2014"
	installed, version := detectCLI(name)

	if !installed {
		return cliStatus{
			CLI:         name,
			Installed:   "\u2717",
			Version:     "\u2014",
			NervAdapter: dash,
			Drift:       dash,
			LastInstall: dash,
		}
	}

	adapterStatus := "not installed"
	drift := dash
	lastInstall := dash

	// A CLI without a registered adapter can be detected but not managed.
	if adapter, ok := adapters.Registry()[name]; ok {
		inst := adapters.NewInstaller()
		if records, err := inst.Journal(name); err == nil && len(records) > 0 {
			adapterStatus = "installed"
			lastInstall = records[len(records)-1].
				Timestamp.Format("2006-01-02 15:04")

			if drifted, err := inst.DetectDrift(adapter); err == nil {
				if drifted {
					drift = "yes"
				} else {
					drift = "no"
				}
			}
		}
	}

	return cliStatus{
		CLI:         name,
		Installed:   "\u2713",
		Version:     version,
		NervAdapter: adapterStatus,
		Drift:       drift,
		LastInstall: lastInstall,
	}
}

// detectCLI checks whether a CLI is present via its registered adapter
// and returns its installed state and version string. CLIs without an
// adapter are reported as not installed.
func detectCLI(name string) (installed bool, version string) {
	adapter, ok := adapters.Registry()[name]
	if !ok {
		return false, ""
	}
	dr, err := adapter.Detect()
	if err != nil || !dr.Found {
		return false, ""
	}
	return true, dr.Info.Version
}
