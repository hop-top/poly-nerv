package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"hop.top/kit/go/console/cli"

	"hop.top/nerv/internal/hookpkg"
)

func addCmd() *cobra.Command {
	var source string

	cmd := &cobra.Command{
		Use:   "add <hook-name>",
		Short: "Add a hook to the project",
		Long: `Install a hook handler into the current project: download (or
copy) the handler from --source (default hop.top/nerv/hooks/<name>),
register it under hooks: in .nerv/hooks.yaml, and pin its resolved
artifact + SHA-256 in .nerv/hooks.lock.

Not idempotent — re-running re-downloads the handler, overwrites the
on-disk binary, and replaces the lockfile entry with a fresh install
timestamp. Use 'nerv remove' first if you want a clean re-install.`,
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			"kit/side-effect":    "write",
			"kit/idempotent":     "no",
			"kit/top-level-verb": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if source == "" {
				source = fmt.Sprintf(
					"https://hop.top/nerv/hooks/%s", name,
				)
			}

			if cli.IsDryRun(cmd) {
				return renderAddPlan(cmd, name, source)
			}

			if err := hookpkg.Install(name, source); err != nil {
				return fmt.Errorf("add hook %q: %w", name, err)
			}

			return renderResult(hookResult{
				Hook:   name,
				Action: "installed",
				Source: source,
			})
		},
	}

	cmd.Flags().StringVar(
		&source, "source", "",
		"URL or local path to hook handler (default: hop.top/nerv/hooks/<name>)",
	)

	return cmd
}

// renderAddPlan emits the structured dry-run preview for `nerv add`:
// the exact files an install would create or rewrite, with nothing
// applied. Targets mirror hookpkg.Install step for step.
func renderAddPlan(cmd *cobra.Command, name, source string) error {
	dirs, err := hookpkg.ResolveDirs(name)
	if err != nil {
		return err
	}
	return renderPlan(cmd,
		map[string]any{"hook": name, "source": source},
		[]cli.Effect{
			{
				Kind:       "create",
				Target:     filepath.Join(dirs.DataDir, "handler"),
				Reversible: true,
				Detail:     "fetch handler from " + source,
			},
			{
				Kind:       "update",
				Target:     dirs.HooksYAML,
				Reversible: true,
				Detail:     fmt.Sprintf("register hook %q (on: tool_call)", name),
			},
			{
				Kind:       "update",
				Target:     dirs.HooksLock,
				Reversible: true,
				Detail:     "pin resolved source + sha256",
			},
		},
	)
}
