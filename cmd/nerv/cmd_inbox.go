//go:build !windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/nerv/internal/inbox"
)

// inboxStateDir returns the local delivery surface directory: the
// inbox/ subtree of the state root (segregated from events/ and
// sessions/; see docs/architecture/crtx-consumer.md).
func inboxStateDir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "nerv", "inbox")
}

func inboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "Read the local message inbox",
	}
	cmd.AddCommand(inboxListCmd(), inboxShowCmd())
	return cmd
}

// inboxRow is one inbox entry rendered for the list view.
type inboxRow struct {
	MsgID   string `table:"Message"  json:"msg_id"  yaml:"msg_id"`
	Time    string `table:"Received" json:"time"    yaml:"time"`
	From    string `table:"From"     json:"from"    yaml:"from"`
	Role    string `table:"Role"     json:"role"    yaml:"role"`
	Preview string `table:"Preview"  json:"preview" yaml:"preview"`
}

func inboxListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List delivered inbox messages, newest first",
		Long: `List messages delivered to the local inbox — foreign agents' turns
routed here by the daemon's crtx consumer. Entries live as plain JSON
under $XDG_STATE_HOME/nerv/inbox/ and are equally readable with jq or
an editor; this view is the quick triage surface.`,
		Annotations: map[string]string{
			"kit/side-effect": "read",
			"kit/idempotent":  "yes",
		},
		RunE: runInboxList,
	}
}

func runInboxList(_ *cobra.Command, _ []string) error {
	entries, err := inbox.NewStore(inboxStateDir()).List()
	if err != nil {
		return fmt.Errorf("read inbox: %w", err)
	}

	rows := make([]inboxRow, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, inboxRow{
			MsgID:   e.MsgID,
			Time:    e.ReceivedAt.Local().Format("2006-01-02 15:04:05"),
			From:    entrySender(e),
			Role:    e.Role,
			Preview: e.Preview,
		})
	}

	format := viper.GetString("format")
	if format == "" {
		format = output.Table
	}
	return output.Render(os.Stdout, format, rows)
}

// entrySender picks the most specific attribution the entry carries:
// the foreign envelope's source kind (scan path), else the bus
// publisher (bus path).
func entrySender(e inbox.Entry) string {
	if e.Origin.SourceKind != "" {
		return e.Origin.SourceKind
	}
	if e.Origin.BusSource != "" {
		return e.Origin.BusSource
	}
	return "unknown"
}

func inboxShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <msg-id>",
		Short: "Show one inbox message in full",
		Long: `Print the complete inbox entry document for a message id from
` + "`nerv inbox list`" + `: attribution, lineage correlation, and the foreign
turn's content parts, verbatim JSON.`,
		Args: cobra.ExactArgs(1),
		Annotations: map[string]string{
			"kit/side-effect": "read",
			"kit/idempotent":  "yes",
		},
		RunE: runInboxShow,
	}
}

func runInboxShow(_ *cobra.Command, args []string) error {
	entry, err := inbox.NewStore(inboxStateDir()).Get(args[0])
	if err != nil {
		if errors.Is(err, inbox.ErrNotFound) {
			e := output.NotFoundError(
				fmt.Sprintf("inbox message %q not found", args[0]))
			e.SuggestedFix = "run `nerv inbox list` to see delivered " +
				"message ids, then re-run `nerv inbox show <msg-id>`"
			return e
		}
		return fmt.Errorf("read inbox: %w", err)
	}

	doc, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("encode entry: %w", err)
	}
	_, err = fmt.Fprintln(os.Stdout, string(doc))
	return err
}
