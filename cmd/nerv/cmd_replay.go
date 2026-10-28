//go:build !windows

package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/dispatch"
	"hop.top/nerv/internal/eventlog"
	"hop.top/nerv/internal/merger"
)

// replayRow shows original vs current decision for a replayed event.
type replayRow struct {
	EventID  string `table:"Event ID" json:"event_id" yaml:"event_id"`
	Event    string `table:"Event"    json:"event"    yaml:"event"`
	Original string `table:"Original" json:"original" yaml:"original"`
	Current  string `table:"Current"  json:"current"  yaml:"current"`
	Changed  string `table:"Changed"  json:"changed"  yaml:"changed"`
	TimeMs   int64  `table:"Time(ms)" json:"time_ms"  yaml:"time_ms"`
}

func replayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay [event-id]",
		Short: "Replay a recorded event through the hook pipeline",
		Long: `Replay recorded events through the current dispatch config:

  nerv replay <event-id>    # replay single event
  nerv replay --last 10     # replay last 10 events

Shows original vs current decision to detect config drift.`,
		RunE: runReplay,
		Annotations: map[string]string{
			"kit/side-effect":    "read",
			"kit/idempotent":     "yes",
			"kit/top-level-verb": "true",
		},
	}

	cmd.Flags().Int("last", 0, "Replay last N events")

	return cmd
}

func runReplay(cmd *cobra.Command, args []string) error {
	// Local --last flag is never bound to the package-global viper
	// (kit uses a private viper instance for persistent flags);
	// read it directly off cmd.Flags(). --format remains a kit
	// persistent flag and stays on viper.
	lastN, _ := cmd.Flags().GetInt("last")

	if len(args) == 0 && lastN == 0 {
		return fmt.Errorf("specify an event-id or use --last N")
	}

	// Load current config.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	cached := config.Compile(cfg)
	matcher := dispatch.NewMatcher()
	runner := dispatch.NewRunner()

	// Fetch entries from the log.
	logger := &eventlog.JSONLLogger{}
	var entries []eventlog.Entry

	if lastN > 0 {
		entries, err = logger.Query(eventlog.Filter{Limit: lastN})
		if err != nil {
			return fmt.Errorf("querying log: %w", err)
		}
	} else {
		// Find by event ID.
		all, qErr := logger.Query(eventlog.Filter{})
		if qErr != nil {
			return fmt.Errorf("querying log: %w", qErr)
		}
		targetID := args[0]
		for _, e := range all {
			if e.Envelope != nil && e.Envelope.EventID == targetID {
				entries = append(entries, e)
				break
			}
		}
		if len(entries) == 0 {
			return replayNotFound(targetID, all)
		}
	}

	// Replay each entry.
	var rows []replayRow
	for _, e := range entries {
		if e.Envelope == nil {
			continue
		}

		// Extract original decision.
		origAction := extractAction(e.Decision)

		// Re-dispatch through current config.
		matched := matcher.Match(cached, e.Envelope)
		var currentAction string
		var timeMs int64

		if len(matched.Hooks) == 0 {
			currentAction = "allow"
		} else {
			decisions := runner.RunHooks(
				matched.Hooks, e.Envelope, cached.Config,
			)
			merged := merger.Merge(decisions)
			currentAction = string(merged.Action)
		}

		changed := "no"
		if origAction != currentAction {
			changed = "YES"
		}

		rows = append(rows, replayRow{
			EventID:  e.Envelope.EventID,
			Event:    e.Envelope.Event,
			Original: origAction,
			Current:  currentAction,
			Changed:  changed,
			TimeMs:   timeMs,
		})
	}

	format := viper.GetString("format")
	if format == "" {
		format = output.Table
	}

	if len(rows) == 0 {
		if format == output.Table {
			_, _ = fmt.Fprintln(os.Stdout, "No events to replay.")
		}
		return nil
	}

	return output.Render(os.Stdout, format, rows)
}

// maxReplayAlternatives caps how many recorded event ids the
// not-found envelope offers as concrete retry commands.
const maxReplayAlternatives = 5

// replayNotFound builds the corrective NOT_FOUND envelope for a
// replay target that is not in the event log. Instead of a dead end,
// the envelope carries a recovery path: concrete replay commands for
// recorded event ids (closest ids first) or, on an empty log, the
// discovery commands that populate it.
func replayNotFound(targetID string, all []eventlog.Entry) *output.Error {
	e := output.NotFoundError(
		fmt.Sprintf("event %q not found in log", targetID))

	ids := make([]string, 0, len(all))
	seen := make(map[string]bool, len(all))
	for _, entry := range all {
		if entry.Envelope == nil || entry.Envelope.EventID == "" {
			continue
		}
		if id := entry.Envelope.EventID; !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		e.Cause = "the event log has no recorded events yet"
		e.SuggestedFix = "run `nerv tail` to watch events being recorded, " +
			"or `nerv replay --last N` once events exist; " +
			"`nerv status` shows the event-log location"
		return e
	}

	sort.SliceStable(ids, func(i, j int) bool {
		return commonPrefixLen(ids[i], targetID) > commonPrefixLen(ids[j], targetID)
	})
	if len(ids) > maxReplayAlternatives {
		ids = ids[:maxReplayAlternatives]
	}
	e.SuggestedFix = "replay a recorded event id (closest matches listed " +
		"as alternatives) or run `nerv replay --last 1` for the most recent event"
	for _, id := range ids {
		e.Alternatives = append(e.Alternatives, "nerv replay "+id)
	}
	return e
}

// commonPrefixLen returns the length of the shared prefix of a and b —
// a cheap nearest-id ranking that keeps typo'd ids near the top.
func commonPrefixLen(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

// extractAction pulls the action string from a logged decision.
func extractAction(decision any) string {
	if decision == nil {
		return "allow"
	}

	switch d := decision.(type) {
	case map[string]any:
		if a, ok := d["action"]; ok {
			return fmt.Sprintf("%v", a)
		}
	case merger.Decision:
		return string(d.Action)
	}

	return "allow"
}
