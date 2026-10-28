package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/nerv/internal/eventlog"
)

// tailRow is a single event log entry rendered for the tail view.
type tailRow struct {
	Time     string `table:"Time"     json:"time"     yaml:"time"`
	Event    string `table:"Event"    json:"event"    yaml:"event"`
	CLI      string `table:"CLI"      json:"cli"      yaml:"cli"`
	Decision string `table:"Decision" json:"decision" yaml:"decision"`
	Duration string `table:"Duration" json:"duration" yaml:"duration"`
}

func tailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tail",
		Short: "Stream live hook events",
		Long: `Tail the JSONL event log in real time:

  nerv tail                        # follow mode
  nerv tail --event PreToolUse     # filter by event
  nerv tail --cli claude           # filter by CLI
  nerv tail --slow                 # only events >100ms`,
		RunE: runTail,
		Annotations: map[string]string{
			"kit/side-effect":    "interactive",
			"kit/idempotent":     "yes",
			"kit/top-level-verb": "true",
		},
	}

	cmd.Flags().String("event", "", "Filter by event name")
	cmd.Flags().String("cli", "", "Filter by CLI name")
	cmd.Flags().Bool("slow", false, "Only show events where handler took >100ms")
	cmd.Flags().Int("last", 20, "Number of recent entries to show initially")

	return cmd
}

func runTail(cmd *cobra.Command, _ []string) error {
	// Local flags live on the cobra command and are never bound to
	// viper (kit's persistent flags use a private viper instance);
	// read them directly off cmd.Flags() instead of the unbound
	// package-global viper. --format remains a kit persistent flag.
	eventFilter, _ := cmd.Flags().GetString("event")
	cliFilter, _ := cmd.Flags().GetString("cli")
	slowOnly, _ := cmd.Flags().GetBool("slow")
	last, _ := cmd.Flags().GetInt("last")

	dir := eventLogDir()

	format := viper.GetString("format")
	if format == "" {
		format = output.Table
	}

	// Show recent entries first.
	logger := &eventlog.JSONLLogger{}
	entries, err := logger.Query(eventlog.Filter{
		Event: eventFilter,
		CLI:   cliFilter,
		Limit: last,
	})
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("querying log: %w", err)
	}

	for _, e := range entries {
		row := entryToRow(e)
		if slowOnly && !isSlow(e) {
			continue
		}
		if format == output.Table {
			printTailLine(row)
		} else {
			renderRow(format, row)
		}
	}

	// Follow mode: watch the current day's file.
	return followLog(dir, eventFilter, cliFilter, slowOnly, format)
}

// followLog tails the current day's log file, rolling over at midnight.
func followLog(
	dir, eventFilter, cliFilter string,
	slowOnly bool,
	format string,
) error {
	for {
		today := time.Now().Format("2006-01-02")
		path := filepath.Join(dir, fmt.Sprintf("events-%s.jsonl", today))

		// Wait for file to exist.
		for {
			if _, err := os.Stat(path); err == nil {
				break
			}
			time.Sleep(500 * time.Millisecond)

			// Check for date rollover.
			newToday := time.Now().Format("2006-01-02")
			if newToday != today {
				today = newToday
				path = filepath.Join(
					dir,
					fmt.Sprintf("events-%s.jsonl", today),
				)
			}
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("opening log: %w", err)
		}

		// Seek to end for follow mode.
		if _, err := f.Seek(0, 2); err != nil {
			_ = f.Close() // read-only; the seek error is the one to report
			return fmt.Errorf("seeking: %w", err)
		}

		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

		for {
			for sc.Scan() {
				var e eventlog.Entry
				if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
					continue
				}

				if !matchesFilter(e, eventFilter, cliFilter) {
					continue
				}
				if slowOnly && !isSlow(e) {
					continue
				}

				row := entryToRow(e)
				if format == output.Table {
					printTailLine(row)
				} else {
					renderRow(format, row)
				}
			}

			// Check for date rollover.
			newToday := time.Now().Format("2006-01-02")
			if newToday != today {
				_ = f.Close() // read-only; nothing to flush
				break         // outer loop picks up new file
			}

			time.Sleep(200 * time.Millisecond)
		}
	}
}

func entryToRow(e eventlog.Entry) tailRow {
	row := tailRow{
		Time:  "—",
		Event: "—",
		CLI:   "—",
	}

	if e.Envelope != nil {
		row.Time = e.Envelope.Timestamp.Format("15:04:05.000")
		row.Event = e.Envelope.Event
		row.CLI = e.Envelope.CLI.Name
	}

	if e.Decision != nil {
		if d, ok := e.Decision.(map[string]any); ok {
			if a, ok := d["action"]; ok {
				row.Decision = fmt.Sprintf("%v", a)
			}
		}
	}

	if e.Timings != nil {
		if total, ok := e.Timings["total_ms"]; ok {
			row.Duration = fmt.Sprintf("%dms", total)
		}
	}

	return row
}

func matchesFilter(e eventlog.Entry, event, cli string) bool {
	if e.Envelope == nil {
		return false
	}
	if event != "" && e.Envelope.Event != event {
		return false
	}
	if cli != "" && e.Envelope.CLI.Name != cli {
		return false
	}
	return true
}

func isSlow(e eventlog.Entry) bool {
	if e.Timings == nil {
		return false
	}
	total, ok := e.Timings["total_ms"]
	return ok && total > 100
}

func printTailLine(r tailRow) {
	parts := []string{r.Time, r.Event, r.CLI}
	if r.Decision != "" {
		parts = append(parts, r.Decision)
	}
	if r.Duration != "" {
		parts = append(parts, r.Duration)
	}
	fmt.Println(strings.Join(parts, "  "))
}

func renderRow(format string, r tailRow) {
	_ = output.Render(os.Stdout, format, r)
}
