package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/core/xdg"

	"hop.top/nerv/adapters"
)

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install nerv adapters for AI CLIs",
		Long: `Manage nerv adapter installation for AI CLIs.

'nerv install add <cli>' writes the adapter's hook config into the
CLI's settings (backing up and journaling the change); run it with
no CLI name to install for every detected CLI. 'nerv install status'
reports installation state per CLI. Safe to re-run: existing adapters
are re-asserted, not duplicated.`,
	}
}

func installAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add [cli]",
		Short: "Install the nerv adapter for an AI CLI",
		Long: `Install the nerv adapter for the named AI CLI by merging its
hook config into the CLI's native settings and recording a journal
entry. Run without arguments to install adapters for every detected
CLI. Safe to re-run: existing adapters are re-asserted, not
duplicated.

Transactional: backs up the existing config, writes atomically,
verifies the post-write hash, rolls back on mismatch, and records
the install in the journal under the nerv state dir.`,
		Args: cobra.MaximumNArgs(1),
		Annotations: map[string]string{
			"kit/side-effect":    "write",
			"kit/idempotent":     "yes",
			"kit/top-level-verb": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if cli.IsDryRun(cmd) {
				return renderInstallAddPlan(cmd, args)
			}
			return runInstall(args)
		},
	}
}

// runInstall installs the nerv adapter for one named CLI, or for every
// detected CLI when no name is given.
func runInstall(args []string) error {
	reg := adapters.Registry()
	inst := adapters.NewInstaller()

	if len(args) == 1 {
		name := args[0]
		adapter, ok := reg[name]
		if !ok {
			return fmt.Errorf(
				"no nerv adapter for %q (supported: %v)",
				name, adapters.SupportedCLIs(),
			)
		}
		return installOne(inst, name, adapter)
	}

	// No arg: install for every detected CLI.
	names := make([]string, 0, len(reg))
	for name := range reg {
		names = append(names, name)
	}
	sort.Strings(names)

	var installed, skipped int
	for _, name := range names {
		adapter := reg[name]
		dr, err := adapter.Detect()
		if err != nil {
			return fmt.Errorf("detect %q: %w", name, err)
		}
		if !dr.Found {
			skipped++
			continue
		}
		if err := installOne(inst, name, adapter); err != nil {
			return err
		}
		installed++
	}

	_, _ = fmt.Fprintf(os.Stdout,
		"installed %d adapter(s), skipped %d undetected CLI(s)\n",
		installed, skipped,
	)
	return nil
}

// installOne runs the transactional install for a single adapter and
// reports the outcome.
func installOne(
	inst *adapters.Installer,
	name string,
	adapter adapters.HookAdapter,
) error {
	if err := inst.Install(adapter, false); err != nil {
		return fmt.Errorf("install %q: %w", name, err)
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s: adapter installed\n", name)
	return nil
}

// renderInstallAddPlan emits the structured dry-run preview for
// `nerv install add`: the config file each detected CLI would have
// its hook config merged into, plus the backup + journal writes the
// transactional install performs, with nothing applied — not even
// the nerv state directory is created. Detection is the same
// read-only probe the real run starts with.
func renderInstallAddPlan(cmd *cobra.Command, args []string) error {
	reg := adapters.Registry()

	var names []string
	planArgs := map[string]any{"cli": "(all detected)"}
	if len(args) == 1 {
		name := args[0]
		if _, ok := reg[name]; !ok {
			return fmt.Errorf(
				"no nerv adapter for %q (supported: %v)",
				name, adapters.SupportedCLIs(),
			)
		}
		names = []string{name}
		planArgs["cli"] = name
	} else {
		for name := range reg {
			names = append(names, name)
		}
		sort.Strings(names)
	}

	// Pattern-construction only; the plan never touches the dir.
	stateDir, stateErr := xdg.RawStateDir("nerv")

	var effects []cli.Effect
	var warnings []string
	for _, name := range names {
		adapter := reg[name]
		dr, err := adapter.Detect()
		if err != nil {
			return fmt.Errorf("detect %q: %w", name, err)
		}
		if !dr.Found {
			if len(args) == 1 {
				warnings = append(warnings, fmt.Sprintf(
					"cli %q not detected; install would fail", name))
			} else {
				warnings = append(warnings, fmt.Sprintf(
					"cli %q not detected; install would skip it", name))
			}
			continue
		}
		paths := adapter.ConfigPaths()
		if len(paths) == 0 {
			warnings = append(warnings, fmt.Sprintf(
				"adapter %q returned no config paths; install would fail", name))
			continue
		}
		effects = append(effects, cli.Effect{
			Kind:       "update",
			Target:     paths[0],
			Reversible: true,
			Detail: fmt.Sprintf(
				"merge nerv adapter hook config for %q (backed-up, hash-verified atomic write)",
				name),
		})
		if stateErr == nil {
			effects = append(effects,
				cli.Effect{
					Kind:       "create",
					Target:     filepath.Join(stateDir, "backups", name),
					Reversible: true,
					Detail:     "timestamped backup of the existing config",
				},
				cli.Effect{
					Kind:       "update",
					Target:     filepath.Join(stateDir, "installs.jsonl"),
					Reversible: true,
					Detail:     "append install journal record",
				},
			)
		}
	}

	return renderPlan(cmd, planArgs, effects, warnings...)
}
