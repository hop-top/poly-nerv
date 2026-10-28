//go:build !windows

package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/nerv/internal/inbox"
)

func seedInbox(t *testing.T, state string) *inbox.Store {
	t.Helper()
	store := inbox.NewStore(filepath.Join(state, "nerv", "inbox"))
	entries := []inbox.Entry{
		{
			SchemaVersion: inbox.EntrySchemaVersion,
			MsgID:         "m-old",
			InboxID:       inbox.InboxForeignTurns,
			Kind:          inbox.KindForeignTurn,
			ReceivedAt:    time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC),
			Origin:        inbox.EntryOrigin{Transport: "scan", SourceKind: "stem", SourceVersion: "0.1.0"},
			EnvelopeID:    "env-a",
			TurnID:        "t-1",
			Role:          "user",
			Content:       []json.RawMessage{json.RawMessage(`{"type":"text","text":"older message"}`)},
			Preview:       "older message",
		},
		{
			SchemaVersion: inbox.EntrySchemaVersion,
			MsgID:         "m-new",
			InboxID:       inbox.InboxForeignTurns,
			Kind:          inbox.KindForeignTurn,
			ReceivedAt:    time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC),
			Origin:        inbox.EntryOrigin{Transport: "bus", Topic: "crtx.turn.user.received", BusSource: "stem"},
			EnvelopeID:    "env-b",
			TurnID:        "t-2",
			Role:          "assistant",
			Content:       []json.RawMessage{json.RawMessage(`{"type":"text","text":"newer message"}`)},
			Preview:       "newer message",
		},
	}
	for _, e := range entries {
		if delivered, err := store.Put(e); err != nil || !delivered {
			t.Fatalf("seed %s: delivered=%v err=%v", e.MsgID, delivered, err)
		}
	}
	return store
}

func TestInboxList_JSON(t *testing.T) {
	bin := buildNerv(t)
	state := t.TempDir()
	seedInbox(t, state)

	cmd := exec.Command(bin, "inbox", "list", "--format", "json")
	cmd.Env = append(cmd.Environ(), "XDG_STATE_HOME="+state)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inbox list: %v\n%s", err, out)
	}

	var rows []struct {
		MsgID   string `json:"msg_id"`
		From    string `json:"from"`
		Role    string `json:"role"`
		Preview string `json:"preview"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("list output not JSON: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2\n%s", len(rows), out)
	}
	// Newest first.
	if rows[0].MsgID != "m-new" || rows[1].MsgID != "m-old" {
		t.Errorf("order = %s,%s; want m-new,m-old", rows[0].MsgID, rows[1].MsgID)
	}
	if rows[0].From != "stem" || rows[0].Preview != "newer message" {
		t.Errorf("row[0] = %+v", rows[0])
	}
}

func TestInboxList_EmptyStateIsCleanExit(t *testing.T) {
	bin := buildNerv(t)
	cmd := exec.Command(bin, "inbox", "list", "--format", "json")
	cmd.Env = append(cmd.Environ(), "XDG_STATE_HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inbox list on empty state must succeed: %v\n%s", err, out)
	}
}

func TestInboxShow_PrintsFullEntry(t *testing.T) {
	bin := buildNerv(t)
	state := t.TempDir()
	seedInbox(t, state)

	cmd := exec.Command(bin, "inbox", "show", "m-new")
	cmd.Env = append(cmd.Environ(), "XDG_STATE_HOME="+state)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inbox show: %v\n%s", err, out)
	}

	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("show output not a JSON document: %v\n%s", err, out)
	}
	if doc["msg_id"] != "m-new" || doc["envelope_id"] != "env-b" {
		t.Errorf("show doc = %v", doc)
	}
	if _, ok := doc["content"]; !ok {
		t.Error("show doc missing content parts")
	}

	// Unknown id: non-zero exit with a useful message.
	cmd = exec.Command(bin, "inbox", "show", "m-missing")
	cmd.Env = append(cmd.Environ(), "XDG_STATE_HOME="+state)
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("show of missing entry succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "not found") {
		t.Errorf("missing-entry error not surfaced:\n%s", out)
	}
}
