package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"hop.top/kit/go/console/cli"
)

const (
	upgradeInstallCmd  = "go install hop.top/nerv/cmd/nerv@latest"
	upgradeReleasesURL = "https://github.com/hop-top/poly-nerv/releases"
)

func upgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Show the installed version and how to upgrade",
		Long: `Print the installed nerv version and the command that upgrades it.

nerv does not update itself: no version check runs and no binary is
replaced. Upgrade by reinstalling through the channel you installed
from, e.g.:

  ` + upgradeInstallCmd,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cli.IsDryRun(cmd) {
				return renderPlan(cmd, nil, nil,
					"upgrade only prints instructions; it applies no changes")
			}
			w := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(w, "nerv %s\n\n", version)
			_, _ = fmt.Fprintln(w, "nerv does not update itself. To upgrade, reinstall:")
			_, _ = fmt.Fprintf(w, "  %s\n", upgradeInstallCmd)
			_, _ = fmt.Fprintf(w, "Releases: %s\n", upgradeReleasesURL)
			return nil
		},
		Annotations: map[string]string{
			"kit/side-effect":    "read",
			"kit/idempotent":     "yes",
			"kit/top-level-verb": "true",
		},
	}
}
