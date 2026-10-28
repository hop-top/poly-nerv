package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"hop.top/kit/go/console/cli"

	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/hookpkg/disable"
)

// disableCmd is the parent group for the three disable sub-verbs.
// Marked top-level-verb so kit's strict validator accepts it at depth
// 1. The sub-leaves carry their own side-effect / idempotent tags.
func disableCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable hooks, events, or hook×event bindings",
		Long: `Disable units of hook dispatch at three granularities:

  • hooks    — set disabled: true on a hook entry in .nerv/hooks.yaml
               (short-circuits every event the hook subscribes to).
  • events   — add an event name to .nerv/disabled.yaml so all hooks
               bound to it are skipped, regardless of their own state.
  • bindings — add a {hook, event} pair to .nerv/disabled.yaml so one
               hook stays live on most events but skips a single one.

All sub-modes are idempotent — re-disabling an already-disabled name
is a no-op. Daemon-side enforcement of the disabled state lands
separately; this verb only writes the configuration. Pair
with 'nerv enable' to reverse.`,
		Annotations: map[string]string{
			"kit/top-level-verb": "true",
		},
	}
	cmd.AddCommand(
		disableHooksCmd(),
		disableEventsCmd(),
		disableBindingsCmd(),
	)
	return cmd
}

func disableHooksCmd() *cobra.Command {
	var noConfirm bool
	cmd := &cobra.Command{
		Use:   "hooks <name>...",
		Short: "Disable one or more hooks by name",
		Long: `Set disabled: true on each named hook entry in .nerv/hooks.yaml.
Hook names must exactly match the name: field; fuzzy matching lives
in the slash command, not the verb. Names that don't match any entry
are reported as unknown and the matching subset is still applied.

Idempotent: hooks already disabled contribute no change. The yaml
file's comments and formatting are preserved (node-level edit).`,
		Args: cobra.MinimumNArgs(1),
		Annotations: map[string]string{
			"kit/side-effect": "write-local",
			"kit/idempotent":  "conditional",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDisableHooks(cmd, args, noConfirm, true)
		},
	}
	cmd.Flags().BoolVar(&noConfirm, "no-confirm", false,
		"skip the interactive y/N prompt; required for non-TTY use")
	return cmd
}

func disableEventsCmd() *cobra.Command {
	var noConfirm bool
	cmd := &cobra.Command{
		Use:   "events <name>...",
		Short: "Disable one or more events",
		Long: `Add each event name to the events: list in .nerv/disabled.yaml.
Creates the sidecar with version: 1.0-draft when it doesn't exist.
Every hook bound to a disabled event is short-circuited regardless
of its own disabled state.

Idempotent — events already in the list contribute no change.`,
		Args: cobra.MinimumNArgs(1),
		Annotations: map[string]string{
			"kit/side-effect": "write-local",
			"kit/idempotent":  "conditional",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDisableEvents(cmd, args, noConfirm, true)
		},
	}
	cmd.Flags().BoolVar(&noConfirm, "no-confirm", false,
		"skip the interactive y/N prompt; required for non-TTY use")
	return cmd
}

func disableBindingsCmd() *cobra.Command {
	var noConfirm bool
	cmd := &cobra.Command{
		Use:   "bindings <hook>:<event>...",
		Short: "Disable one or more hook×event bindings",
		Long: `Add each <hook>:<event> pair to the bindings: list in
.nerv/disabled.yaml. Finer granularity than disabling the whole hook
or the whole event — useful when one hook should stay live on most
events but skip a specific one.

Syntax: <hook>:<event>, one pair per positional arg. The colon
is the delimiter; hooks and events may not contain ':'. Idempotent.`,
		Args: cobra.MinimumNArgs(1),
		Annotations: map[string]string{
			"kit/side-effect": "write-local",
			"kit/idempotent":  "conditional",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDisableBindings(cmd, args, noConfirm, true)
		},
	}
	cmd.Flags().BoolVar(&noConfirm, "no-confirm", false,
		"skip the interactive y/N prompt; required for non-TTY use")
	return cmd
}

// parseBindings turns each "<hook>:<event>" arg into a Binding.
// Errors on missing colon, empty hook/event token, embedded ':' in
// either side, or an event name not in the canonical catalog.
func parseBindings(args []string) ([]disable.Binding, error) {
	out := make([]disable.Binding, 0, len(args))
	for _, raw := range args {
		idx := strings.Index(raw, ":")
		if idx <= 0 || idx == len(raw)-1 {
			return nil, fmt.Errorf("binding %q: want <hook>:<event>", raw)
		}
		hook := raw[:idx]
		event := raw[idx+1:]
		// Reject a second ':' anywhere in the remainder — the
		// catalog has no such names; tolerating it would silently
		// truncate the event token.
		if strings.Contains(event, ":") {
			return nil, fmt.Errorf(
				"binding %q: hook/event tokens may not contain ':'", raw)
		}
		if !events.IsValid(events.Event(event)) {
			return nil, fmt.Errorf(
				"binding %q: unknown event %q (see references/event-catalog.md)",
				raw, event)
		}
		out = append(out, disable.Binding{
			Hook:  hook,
			Event: event,
		})
	}
	return out, nil
}

// confirm prompts y/N on the controlling tty. Auto-skips when
// noConfirm is true; refuses when stdin isn't a tty and noConfirm
// is unset (mirrors kit's destructive-gate policy).
func confirm(prompt string, noConfirm bool) error {
	if noConfirm {
		return nil
	}
	if !stdinIsTTY() {
		return fmt.Errorf(
			"refusing to proceed without confirmation in non-TTY context; pass --no-confirm")
	}
	fmt.Fprint(os.Stderr, prompt)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("read confirmation: %w", err)
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "y" && line != "yes" {
		return fmt.Errorf("aborted")
	}
	return nil
}

// stdinIsTTY reports whether os.Stdin is connected to a terminal.
// Use os.ModeCharDevice on the fd's Stat — same trick kit's policy
// middleware uses for its tty-detection arm.
func stdinIsTTY() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}

func runDisableHooks(cmd *cobra.Command, names []string, noConfirm, doDisable bool) error {
	proj, err := disable.OpenProject()
	if err != nil {
		return err
	}
	if cli.IsDryRun(cmd) {
		return renderHooksTogglePlan(cmd, proj, names, doDisable)
	}
	verb := "disable"
	if !doDisable {
		verb = "enable"
	}
	prompt := fmt.Sprintf("About to %s the following hooks:\n", verb)
	for _, n := range names {
		prompt += "  - " + n + "\n"
	}
	prompt += "Continue? [y/N] "
	if err := confirm(prompt, noConfirm); err != nil {
		return err
	}

	var changed, unknown []string
	if doDisable {
		changed, unknown, err = proj.DisableHooks(names)
	} else {
		changed, unknown, err = proj.EnableHooks(names)
	}
	if err != nil {
		return err
	}
	reportNameChange(verb, "hooks", changed, unknown)
	return nil
}

func runDisableEvents(cmd *cobra.Command, names []string, noConfirm, doDisable bool) error {
	// Validate against the canonical catalog up-front so a typo
	// doesn't pollute the sidecar with an unmatched entry.
	var unknown []string
	for _, n := range names {
		if !events.IsValid(events.Event(n)) {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf(
			"unknown event(s) %v (see references/event-catalog.md)",
			unknown)
	}
	proj, err := disable.OpenProject()
	if err != nil {
		return err
	}
	if cli.IsDryRun(cmd) {
		return renderEventsTogglePlan(cmd, proj, names, doDisable)
	}
	verb := "disable"
	if !doDisable {
		verb = "enable"
	}
	prompt := fmt.Sprintf("About to %s the following events:\n", verb)
	for _, n := range names {
		prompt += "  - " + n + "\n"
	}
	prompt += "Continue? [y/N] "
	if err := confirm(prompt, noConfirm); err != nil {
		return err
	}

	var diff []string
	if doDisable {
		diff, err = proj.DisableEvents(names)
	} else {
		diff, err = proj.EnableEvents(names)
	}
	if err != nil {
		return err
	}
	reportNameChange(verb, "events", diff, nil)
	return nil
}

func runDisableBindings(cmd *cobra.Command, args []string, noConfirm, doDisable bool) error {
	pairs, err := parseBindings(args)
	if err != nil {
		return err
	}
	proj, err := disable.OpenProject()
	if err != nil {
		return err
	}
	if cli.IsDryRun(cmd) {
		return renderBindingsTogglePlan(cmd, proj, pairs, doDisable)
	}
	verb := "disable"
	if !doDisable {
		verb = "enable"
	}
	prompt := fmt.Sprintf("About to %s the following bindings:\n", verb)
	for _, b := range pairs {
		prompt += "  - " + b.Hook + ":" + b.Event + "\n"
	}
	prompt += "Continue? [y/N] "
	if err := confirm(prompt, noConfirm); err != nil {
		return err
	}

	var diff []disable.Binding
	if doDisable {
		diff, err = proj.DisableBindings(pairs)
	} else {
		diff, err = proj.EnableBindings(pairs)
	}
	if err != nil {
		return err
	}
	reportBindingChange(verb, diff)
	return nil
}

// reportNameChange prints a human-readable summary for hooks/events.
// unknown is only meaningful for hooks (events have no membership check).
func reportNameChange(verb, unit string, changed, unknown []string) {
	pastTense := verb + "d" // disable→disabled, enable→enabled
	if len(changed) == 0 && len(unknown) == 0 {
		fmt.Printf("no %s changed (already %s)\n", unit, pastTense)
		return
	}
	if len(changed) > 0 {
		fmt.Printf("%s %d %s:\n", pastTense, len(changed), unit)
		for _, n := range changed {
			fmt.Println("  -", n)
		}
	}
	if len(unknown) > 0 {
		fmt.Printf("unknown (no matching entry): %d\n", len(unknown))
		for _, n := range unknown {
			fmt.Println("  -", n)
		}
	}
}

// toggleVerb maps the doDisable bool to the verb string shared by
// prompts, reports, and plan details.
func toggleVerb(doDisable bool) string {
	if doDisable {
		return "disable"
	}
	return "enable"
}

// renderHooksTogglePlan emits the dry-run preview for
// `nerv disable|enable hooks` without touching hooks.yaml. Reads the
// current hook list only to split names into matching / unknown so
// the preview mirrors the partial-apply semantics of the real run.
func renderHooksTogglePlan(cmd *cobra.Command, proj *disable.Project, names []string, doDisable bool) error {
	verb := toggleVerb(doDisable)

	known, err := proj.ListHookNames()
	if err != nil {
		// The real run would fail on the same read; surface the
		// doomed outcome as a warning rather than mutating exit
		// semantics of the preview.
		return renderPlan(cmd,
			map[string]any{"hooks": names},
			nil,
			fmt.Sprintf("cannot read %s (%v); %s would fail", proj.HooksYAML, err, verb))
	}
	knownSet := make(map[string]bool, len(known))
	for _, n := range known {
		knownSet[n] = true
	}
	var matching, unknown []string
	for _, n := range names {
		if knownSet[n] {
			matching = append(matching, n)
		} else {
			unknown = append(unknown, n)
		}
	}

	var effects []cli.Effect
	if len(matching) > 0 {
		detail := fmt.Sprintf("set disabled: true on hooks [%s]", strings.Join(matching, ", "))
		if !doDisable {
			detail = fmt.Sprintf("strip disabled: from hooks [%s]", strings.Join(matching, ", "))
		}
		effects = append(effects, cli.Effect{
			Kind:       "update",
			Target:     proj.HooksYAML,
			Reversible: true,
			Detail:     detail,
		})
	}
	var warnings []string
	if len(unknown) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"hooks [%s] match no entry in hooks.yaml; the matching subset still applies",
			strings.Join(unknown, ", ")))
	}
	if len(matching) == 0 {
		warnings = append(warnings,
			fmt.Sprintf("no hooks match; %s converges to a no-op", verb))
	}
	return renderPlan(cmd, map[string]any{"hooks": names}, effects, warnings...)
}

// renderEventsTogglePlan emits the dry-run preview for
// `nerv disable|enable events` without touching disabled.yaml. Loads
// the sidecar read-only to report which names would actually change
// and which converge to a no-op.
func renderEventsTogglePlan(cmd *cobra.Command, proj *disable.Project, names []string, doDisable bool) error {
	verb := toggleVerb(doDisable)
	sc, err := proj.LoadSidecar()
	if err != nil {
		// Same corrupt-sidecar failure the real write path surfaces.
		return err
	}

	var wouldChange, noop []string
	for _, n := range names {
		if sc.HasEvent(n) == doDisable {
			noop = append(noop, n)
		} else {
			wouldChange = append(wouldChange, n)
		}
	}

	var effects []cli.Effect
	if len(wouldChange) > 0 {
		detail := fmt.Sprintf("add events [%s] to events: list", strings.Join(wouldChange, ", "))
		if !doDisable {
			detail = fmt.Sprintf("remove events [%s] from events: list", strings.Join(wouldChange, ", "))
		}
		effects = append(effects, cli.Effect{
			Kind:       "update",
			Target:     proj.DisabledYAML,
			Reversible: true,
			Detail:     detail,
		})
	}
	var warnings []string
	if len(noop) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"events [%s] are already %sd; no change", strings.Join(noop, ", "), verb))
	}
	if len(wouldChange) == 0 {
		warnings = append(warnings,
			fmt.Sprintf("%s converges to a no-op; no files would change", verb))
	}
	return renderPlan(cmd, map[string]any{"events": names}, effects, warnings...)
}

// renderBindingsTogglePlan emits the dry-run preview for
// `nerv disable|enable bindings` without touching disabled.yaml.
func renderBindingsTogglePlan(cmd *cobra.Command, proj *disable.Project, pairs []disable.Binding, doDisable bool) error {
	verb := toggleVerb(doDisable)
	sc, err := proj.LoadSidecar()
	if err != nil {
		return err
	}

	format := func(bs []disable.Binding) string {
		parts := make([]string, len(bs))
		for i, b := range bs {
			parts[i] = b.Hook + ":" + b.Event
		}
		return strings.Join(parts, ", ")
	}

	var wouldChange, noop []disable.Binding
	for _, b := range pairs {
		if sc.HasBinding(b.Hook, b.Event) == doDisable {
			noop = append(noop, b)
		} else {
			wouldChange = append(wouldChange, b)
		}
	}

	var effects []cli.Effect
	if len(wouldChange) > 0 {
		detail := fmt.Sprintf("add bindings [%s] to bindings: list", format(wouldChange))
		if !doDisable {
			detail = fmt.Sprintf("remove bindings [%s] from bindings: list", format(wouldChange))
		}
		effects = append(effects, cli.Effect{
			Kind:       "update",
			Target:     proj.DisabledYAML,
			Reversible: true,
			Detail:     detail,
		})
	}
	var warnings []string
	if len(noop) > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"bindings [%s] are already %sd; no change", format(noop), verb))
	}
	if len(wouldChange) == 0 {
		warnings = append(warnings,
			fmt.Sprintf("%s converges to a no-op; no files would change", verb))
	}
	return renderPlan(cmd, map[string]any{"bindings": format(pairs)}, effects, warnings...)
}

func reportBindingChange(verb string, diff []disable.Binding) {
	pastTense := verb + "d"
	if len(diff) == 0 {
		fmt.Printf("no bindings changed (already %s)\n", pastTense)
		return
	}
	fmt.Printf("%s %d binding(s):\n", pastTense, len(diff))
	for _, b := range diff {
		fmt.Printf("  - %s:%s\n", b.Hook, b.Event)
	}
}
