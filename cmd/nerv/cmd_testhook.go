//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/dispatch"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/events"
)

// testResult is a single test invocation result.
type testResult struct {
	Event    string `table:"Event"    json:"event"    yaml:"event"`
	Hook     string `table:"Hook"     json:"hook"     yaml:"hook"`
	Decision string `table:"Decision" json:"decision" yaml:"decision"`
	TimeMs   int64  `table:"Time(ms)" json:"time_ms"  yaml:"time_ms"`
	Message  string `table:"Message"  json:"message,omitempty" yaml:"message,omitempty"`
}

func testCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test [hook]",
		Short: "Run hook handlers with synthetic envelopes",
		Long: `Send synthetic test envelopes through the dispatch pipeline:

  nerv test --event PreToolUse --tool Bash --command "rm -rf /"
  nerv test --all`,
		RunE: runTest,
		Annotations: map[string]string{
			"kit/side-effect":    "interactive",
			"kit/idempotent":     "no",
			"kit/top-level-verb": "true",
		},
	}

	cmd.Flags().String("event", "", "Event name to test (e.g. PreToolUse)")
	cmd.Flags().String("tool", "", "Tool name for payload")
	cmd.Flags().String("command", "", "Command string for payload")
	cmd.Flags().Bool("all", false, "Run all hooks with sample envelopes")

	return cmd
}

func runTest(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	cached := config.Compile(cfg)
	runner := dispatch.NewRunner()
	matcher := dispatch.NewMatcher()

	// Local flags live on the cobra command and are never bound to
	// viper (kit's persistent flags use a private viper instance);
	// read them directly off cmd.Flags() instead of the unbound
	// package-global viper.
	allFlag, _ := cmd.Flags().GetBool("all")
	eventName, _ := cmd.Flags().GetString("event")
	toolName, _ := cmd.Flags().GetString("tool")
	command, _ := cmd.Flags().GetString("command")

	var results []testResult

	if allFlag {
		results = testAllHooks(cached, matcher, runner)
	} else {
		if eventName == "" {
			return fmt.Errorf("--event or --all required")
		}
		if !events.IsValid(events.Event(eventName)) {
			return fmt.Errorf("unknown event %q", eventName)
		}
		env := buildSyntheticEnvelope(eventName, toolName, command)
		hookResults := testSingleEvent(cached, matcher, runner, env)
		results = append(results, hookResults...)
	}

	format := viper.GetString("format")
	if format == "" {
		format = output.Table
	}

	if len(results) == 0 {
		if format == output.Table {
			_, _ = fmt.Fprintln(os.Stdout, "No matching hooks found.")
		}
		return nil
	}

	return output.Render(os.Stdout, format, results)
}

// buildSyntheticEnvelope creates a test envelope for a given event.
func buildSyntheticEnvelope(
	event, toolName, command string,
) *envelope.Envelope {
	// Canonical payload shape (spec/events.yaml): the command lives
	// under tool_input, exactly as adapters deliver it.
	payload := map[string]any{"session_id": "test-session"}
	if toolName != "" {
		payload["tool_name"] = toolName
	}
	if command != "" {
		payload["tool_input"] = map[string]any{"command": command}
	}

	payloadBytes, _ := json.Marshal(payload)

	src := "nerv-test"
	return &envelope.Envelope{
		NervVersion: version,
		Event:       event,
		EventID:     fmt.Sprintf("test-%d", time.Now().UnixNano()),
		Timestamp:   time.Now(),
		CLI: envelope.CLIInfo{
			Name:    "nerv-test",
			Version: version,
		},
		Session: envelope.SessionInfo{
			ID:  "test-session",
			CWD: mustCwd(),
		},
		Payload:         payloadBytes,
		Synthesized:     true,
		SynthesisSource: &src,
	}
}

// testSingleEvent dispatches a synthetic envelope and collects results.
func testSingleEvent(
	cfg *config.CachedConfig,
	matcher *dispatch.Matcher,
	runner *dispatch.Runner,
	env *envelope.Envelope,
) []testResult {
	matched := matcher.Match(cfg, env)
	if len(matched.Hooks) == 0 {
		return nil
	}

	start := time.Now()
	decisions := runner.RunHooks(
		matched.Hooks, env, cfg.Config,
	)
	elapsed := time.Since(start)

	results := make([]testResult, 0, len(decisions))
	for i, d := range decisions {
		hookName := ""
		if i < len(matched.Hooks) {
			hookName = matched.Hooks[i].Name
		}
		results = append(results, testResult{
			Event:    env.Event,
			Hook:     hookName,
			Decision: string(d.Action),
			TimeMs:   elapsed.Milliseconds(),
			Message:  d.Message,
		})
	}
	return results
}

// testAllHooks runs sample envelopes for every configured event.
func testAllHooks(
	cfg *config.CachedConfig,
	matcher *dispatch.Matcher,
	runner *dispatch.Runner,
) []testResult {
	var results []testResult

	for ev := range cfg.ByEvent {
		env := buildSyntheticEnvelope(ev, "", "")
		r := testSingleEvent(cfg, matcher, runner, env)
		results = append(results, r...)
	}
	return results
}

func mustCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
