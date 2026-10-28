// Package main is the nerv CLI — the full-featured nerv command-line tool.
package main

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"

	// Register LLM provider adapters with kit/ai/llm's DefaultRegistry
	// so `nerv create` can resolve URIs like anthropic://… or
	// openai://… at runtime. Anonymous imports trigger each adapter's
	// init() side-effect.
	_ "hop.top/kit/go/ai/llm/anthropic"
	_ "hop.top/kit/go/ai/llm/ollama"
	_ "hop.top/kit/go/ai/llm/openai"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	root := cli.New(cli.Config{
		Name:    "nerv",
		Version: version,
		Short:   "Universal Hooks Protocol — manage AI CLI hooks",
		Accent:  "#00BFA5",
		// The pre-existing surface sits exactly at kit's default cap
		// of 10 depth-1 verbs; `capabilities` (the factor-5 contract
		// surface) is the deliberate 11th.
		MaxTopLevelVerbs: 11,
	})

	install := installCmd()
	install.AddCommand(installAddCmd())
	install.AddCommand(installStatusCmd())

	root.Cmd.AddCommand(
		install,
		doctorCmd(),
		lintCmd(),
		testCmd(),
		replayCmd(),
		tailCmd(),
		addCmd(),
		createCmd(),
		removeCmd(),
		disableCmd(),
		enableCmd(),
		daemonCmd(),
		inboxCmd(),
		upgradeCmd(),
		statusCmd(),
		capabilitiesCmd(root),
	)

	// Make the root runnable: bare `nerv` keeps printing help, but a
	// runnable root means cobra now validates positional args (the
	// root's Args is kit's cobra.NoArgs), so `nerv frobnicate` fails
	// with "unknown command …" instead of printing help and exiting 0.
	root.Cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	}

	// kit binds global flags (--format, --cols, …) to its own viper
	// instance; every RunE here reads the process-global viper. Mirror
	// the persistent flags into the global instance so flag values
	// actually reach the commands.
	_ = viper.BindPFlags(root.Cmd.PersistentFlags())

	if err := root.Execute(context.Background()); err != nil {
		os.Exit(exitCodeFor(err))
	}
}

// exitCodeFor maps an Execute failure to the kit exit-class table
// (GENERIC=1, USAGE=2, NOT_FOUND=3, CONFLICT=4, UNAUTHORIZED=5, …).
//
// Precedence:
//  1. A structured error envelope (kit RunE middleware wraps every
//     leaf error into *output.Error) declares the authoritative
//     exit_code — the process must exit with exactly that code.
//  2. Context cancellation exits 130 (128+SIGINT), the one signal
//     not modeled as an envelope.
//  3. cobra/pflag parse failures (unknown flag/command, bad args)
//     never reach the RunE middleware; classify them as USAGE=2.
//  4. Everything else is GENERIC=1.
func exitCodeFor(err error) int {
	type asCLIError interface{ AsCLIError() *output.Error }
	var ce asCLIError
	if errors.As(err, &ce) {
		if env := ce.AsCLIError(); env != nil && env.ExitCode != 0 {
			return env.ExitCode
		}
	}
	if errors.Is(err, context.Canceled) {
		return 130
	}
	if isUsageError(err) {
		return 2
	}
	return 1
}

// isUsageError reports whether err is a cobra/pflag parse failure.
// cobra returns bare fmt.Errorf errors for these (no sentinel to
// errors.Is against — see spf13/cobra#2266), so classification is by
// message prefix, mirroring fang's own usage detector plus cobra's
// positional-arg validators.
func isUsageError(err error) bool {
	s := err.Error()
	for _, prefix := range []string{
		"unknown flag:",
		"unknown shorthand flag:",
		"unknown command",
		"flag needs an argument:",
		"invalid argument",
		"accepts ",          // cobra.ExactArgs / MaximumNArgs
		"requires at least", // cobra.MinimumNArgs
	} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
