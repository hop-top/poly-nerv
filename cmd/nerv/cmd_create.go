package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"hop.top/kit/go/console/cli"

	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/hookpkg"
	"hop.top/nerv/internal/hookpkg/create"
)

// createCmd builds the `nerv create <description>` cobra leaf.
//
// Generates a hook handler script from a natural-language intent via an
// LLM provider, then registers it through the same hookpkg.Install path
// that `nerv add` uses. Flags mirror the slash-command spec at
// .agents/commands/nerv-create.md.
func createCmd() *cobra.Command {
	var (
		lang           string
		provider       string
		events         []string
		name           string
		allowUntrusted bool
	)

	cmd := &cobra.Command{
		Use:   "create <description>",
		Short: "Generate a hook from a natural-language intent",
		Long: `Generate a hook handler script and register it via hooks.yaml.

The handler is authored by an LLM provider — pass --llm-provider to
override the default (resolved from $LLM_PROVIDER or ~/.config/hop/
llm.yaml). Pass --lang sh|py to control language; --events <csv>
overrides the inferred event subscription; --dry-run previews the
files an apply would touch without invoking the provider or writing
anything.

LLM-generated handlers run with your privileges when the daemon
dispatches. By default this verb refuses to write the handler to
disk — it prints the generated script + yaml entry so you can audit,
and exits non-zero. Re-run with --allow-untrusted once you've read
the script to commit the write.

Not idempotent — re-running with the same description re-invokes the
LLM, may produce a different script, and replaces any existing entry
of the same name. Use 'nerv remove' first for a clean re-create.`,
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			"kit/side-effect":    "write-local",
			"kit/idempotent":     "no",
			"kit/top-level-verb": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := create.ValidateLang(lang); err != nil {
				return err
			}
			if cli.IsDryRun(cmd) {
				return renderCreatePlan(cmd, args[0], lang, events, name)
			}
			return runCreate(cmd, args[0], lang, provider, events,
				allowUntrusted, name)
		},
	}

	cmd.Flags().StringVar(&lang, "lang", "sh",
		"Handler language: sh | py")
	cmd.Flags().StringVar(&provider, "llm-provider", "",
		"LLM provider URI (default from $LLM_PROVIDER or llm.yaml); "+
			"API keys MUST come from env (ANTHROPIC_API_KEY, OPENAI_API_KEY, ...) "+
			"or llm.yaml — never via ?api_key=... query params (visible in ps + history)")
	cmd.Flags().StringSliceVar(&events, "events", nil,
		"Override inferred event subscription (comma-separated)")
	cmd.Flags().StringVar(&name, "name", "",
		"Override the derived hook name")
	cmd.Flags().BoolVar(&allowUntrusted, "allow-untrusted", false,
		"Commit the LLM-generated handler to disk after auditing the "+
			"printed script. Required for any non-dry-run write — "+
			"acknowledges that the script will execute as your user.")

	return cmd
}

// runCreate is the verb-side orchestrator. Builds the Generator,
// delegates to create.Run, and surfaces a one-line summary on success.
func runCreate(cmd *cobra.Command, intent, lang, provider string,
	events []string, allowUntrusted bool, nameOverride string,
) error {
	gen, err := create.NewGenerator(provider)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}

	report, err := create.Run(cmd.Context(), gen, create.RunOptions{
		Intent:         intent,
		Lang:           lang,
		Events:         events,
		NameOverride:   nameOverride,
		AllowUntrusted: allowUntrusted,
		Stdout:         cmd.OutOrStdout(),
	})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(),
		"hook %q created\n  events:  %v\n  script:  %s\n  yaml:    %s\n",
		report.Name, report.Events, report.ScriptPath, report.HooksYAML,
	)

	// Optional best-effort lint warning — does not fail the verb.
	if _, err := os.Stat(report.HooksYAML); err == nil {
		// We intentionally don't shell out to `nerv lint` here; the
		// caller can invoke it themselves. The strict-validate tests
		// keep the verb fast and side-effect-light.
		_ = err
	}

	return nil
}

// renderCreatePlan emits the structured dry-run preview for
// `nerv create` WITHOUT resolving or invoking the LLM provider: the
// files an apply would create or rewrite, derived from flags and the
// intent alone. The generated script itself only exists after a real
// run — the default (no --allow-untrusted) run prints it for audit
// while still refusing to write.
func renderCreatePlan(cmd *cobra.Command, intent, lang string,
	evts []string, nameOverride string,
) error {
	// Explicit --events must be catalog-valid; the real run rejects
	// unknown names too (after generation), so fail the same way
	// before rendering a plan for a doomed apply.
	var unknown []string
	for _, e := range evts {
		if !events.IsValid(events.Event(e)) {
			unknown = append(unknown, e)
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf(
			"unknown event(s) %v (see references/event-catalog.md)",
			unknown)
	}

	// Name resolution mirrors create.Run minus the provider: --name
	// wins, otherwise derive from the intent.
	name := strings.ToLower(strings.TrimSpace(nameOverride))
	derived := name == ""
	if derived {
		name = create.DeriveName(intent)
	}

	dirs, err := hookpkg.ResolveDirs(name)
	if err != nil {
		return err
	}

	on := "inferred by the provider at apply time"
	if len(evts) > 0 {
		on = strings.Join(evts, ", ")
	}

	warnings := []string{
		"handler content is generated by the LLM provider at apply time; " +
			"a real run prints the script and refuses to write it until --allow-untrusted",
	}
	if derived {
		warnings = append(warnings, fmt.Sprintf(
			"hook name %q is derived from the intent; the provider may suggest a different name at apply time",
			name))
	}

	return renderPlan(cmd,
		map[string]any{"intent": intent, "lang": lang, "hook": name},
		[]cli.Effect{
			{
				Kind:       "create",
				Target:     filepath.Join(dirs.DataDir, "handler"),
				Reversible: true,
				Detail:     fmt.Sprintf("write LLM-generated %s handler", lang),
			},
			{
				Kind:       "update",
				Target:     dirs.HooksYAML,
				Reversible: true,
				Detail:     fmt.Sprintf("register hook %q (on: %s)", name, on),
			},
			{
				Kind:       "update",
				Target:     dirs.HooksLock,
				Reversible: true,
				Detail:     "pin resolved source + sha256",
			},
		},
		warnings...,
	)
}
