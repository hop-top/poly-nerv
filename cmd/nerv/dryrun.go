package main

import (
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// renderPlan emits the kit dry-run Plan for cmd on stdout in the
// active --format. Mutating leaves call this instead of applying
// anything when --dry-run is set: the preview lists the effects the
// command WOULD apply, and the process exits 0 so agents can
// pre-validate before re-issuing without the flag.
func renderPlan(cmd *cobra.Command, args map[string]any, effects []cli.Effect, warnings ...string) error {
	format := viper.GetString("format")
	if format == "" {
		format = output.Table
	}
	return output.RenderPlan(os.Stdout, format, cli.Plan{
		Command:     cmd.CommandPath(),
		Args:        args,
		Effects:     effects,
		Warnings:    warnings,
		GeneratedAt: time.Now().UTC(),
	})
}
