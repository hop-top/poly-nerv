package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"hop.top/kit/go/console/cli"

	"hop.top/nerv/internal/hookpkg"
)

func removeCmd() *cobra.Command {
	var purge bool

	cmd := &cobra.Command{
		Use:   "remove <hook-name>",
		Short: "Remove a hook from the project",
		Long: `Unregister a hook from the current project: strip its entry
from .nerv/hooks.yaml and .nerv/hooks.lock. The on-disk handler under
$XDG_DATA_HOME/nerv/hooks/<name> is left in place by default so a
later 'nerv add' re-registers without re-downloading.

Pass --purge to also rm -rf the handler directory. Destructive, so
kit's confirm policy gates the run; pass --confirm=yes for batch use.`,
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			"kit/side-effect":    "destructive",
			"kit/idempotent":     "yes",
			"kit/top-level-verb": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if cli.IsDryRun(cmd) {
				return renderRemovePlan(cmd, name, purge)
			}

			if err := hookpkg.Remove(name, purge); err != nil {
				return fmt.Errorf("remove hook %q: %w", name, err)
			}

			return renderResult(hookResult{
				Hook:   name,
				Action: "removed",
				Purged: purge,
			})
		},
	}

	cmd.Flags().BoolVar(
		&purge, "purge", false,
		"also delete handler files from disk",
	)

	return cmd
}

// renderRemovePlan emits the structured dry-run preview for
// `nerv remove` without touching hooks.yaml, hooks.lock, or handler
// files. Reads current state only to warn when the removal would be
// a no-op.
func renderRemovePlan(cmd *cobra.Command, name string, purge bool) error {
	dirs, err := hookpkg.ResolveDirs(name)
	if err != nil {
		return err
	}
	effects := []cli.Effect{
		{
			Kind:       "update",
			Target:     dirs.HooksYAML,
			Reversible: true,
			Detail:     fmt.Sprintf("unregister hook %q", name),
		},
		{
			Kind:       "update",
			Target:     dirs.HooksLock,
			Reversible: true,
			Detail:     "drop lockfile entry",
		},
	}
	if purge {
		effects = append(effects, cli.Effect{
			Kind:       "delete",
			Target:     dirs.DataDir,
			Reversible: false,
			Detail:     "purge handler files from disk",
		})
	}

	var warnings []string
	if lf, lfErr := hookpkg.ReadLockfile(dirs.HooksLock); lfErr == nil && !lf.Has(name) {
		warnings = append(warnings,
			fmt.Sprintf("hook %q is not in the lockfile; removal converges to a no-op", name))
	}

	return renderPlan(cmd,
		map[string]any{"hook": name, "purge": purge},
		effects, warnings...)
}
