package main

import "github.com/spf13/cobra"

// enableCmd is the symmetric counterpart to disableCmd. Mirrors the
// three sub-leaves and delegates to the same run* helpers with
// doDisable=false. Splitting enable into its own verb (rather than a
// --enable flag on disable) keeps the discoverability + auditability
// of two top-level verbs at the cost of a tiny amount of duplication.
func enableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable hooks, events, or hook×event bindings",
		Long: `Re-enable units previously disabled by 'nerv disable':

  • hooks    — strip the disabled: field from the entry in
               .nerv/hooks.yaml (re-enables every event the hook
               subscribes to).
  • events   — remove an event name from the events: list in
               .nerv/disabled.yaml.
  • bindings — remove a {hook, event} pair from the bindings: list in
               .nerv/disabled.yaml.

Idempotent: enabling an already-enabled name is a no-op. Symmetric
round-trip with 'nerv disable' restores the original configuration.`,
		Annotations: map[string]string{
			"kit/top-level-verb": "true",
		},
	}
	cmd.AddCommand(
		enableHooksCmd(),
		enableEventsCmd(),
		enableBindingsCmd(),
	)
	return cmd
}

func enableHooksCmd() *cobra.Command {
	var noConfirm bool
	cmd := &cobra.Command{
		Use:   "hooks <name>...",
		Short: "Enable one or more hooks by name",
		Long: `Strip the disabled: field from each named hook entry in
.nerv/hooks.yaml. Names must exactly match the name: field; names
that don't match any entry are reported as unknown and the matching
subset is still applied. Idempotent: hooks already enabled are no-op.`,
		Args: cobra.MinimumNArgs(1),
		Annotations: map[string]string{
			"kit/side-effect": "write-local",
			"kit/idempotent":  "conditional",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDisableHooks(cmd, args, noConfirm, false)
		},
	}
	cmd.Flags().BoolVar(&noConfirm, "no-confirm", false,
		"skip the interactive y/N prompt; required for non-TTY use")
	return cmd
}

func enableEventsCmd() *cobra.Command {
	var noConfirm bool
	cmd := &cobra.Command{
		Use:   "events <name>...",
		Short: "Enable one or more events",
		Long: `Remove each event name from the events: list in
.nerv/disabled.yaml. Idempotent — events not present in the list
are no-op. Empty .nerv/disabled.yaml is left in place.`,
		Args: cobra.MinimumNArgs(1),
		Annotations: map[string]string{
			"kit/side-effect": "write-local",
			"kit/idempotent":  "conditional",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDisableEvents(cmd, args, noConfirm, false)
		},
	}
	cmd.Flags().BoolVar(&noConfirm, "no-confirm", false,
		"skip the interactive y/N prompt; required for non-TTY use")
	return cmd
}

func enableBindingsCmd() *cobra.Command {
	var noConfirm bool
	cmd := &cobra.Command{
		Use:   "bindings <hook>:<event>...",
		Short: "Enable one or more hook×event bindings",
		Long: `Remove each <hook>:<event> pair from the bindings: list in
.nerv/disabled.yaml. Syntax matches 'nerv disable bindings'.
Idempotent — pairs not present in the list are no-op.`,
		Args: cobra.MinimumNArgs(1),
		Annotations: map[string]string{
			"kit/side-effect": "write-local",
			"kit/idempotent":  "conditional",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDisableBindings(cmd, args, noConfirm, false)
		},
	}
	cmd.Flags().BoolVar(&noConfirm, "no-confirm", false,
		"skip the interactive y/N prompt; required for non-TTY use")
	return cmd
}
