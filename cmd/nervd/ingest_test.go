package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/crtx"
	"hop.top/nerv/internal/inbox"
)

func waitForEntries(t *testing.T, store *inbox.Store, want int) []inbox.Entry {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		entries, err := store.List()
		if err == nil && len(entries) >= want {
			return entries
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d inbox entries; have %d (err=%v)",
				want, len(entries), err)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestStartIngest_BusToInbox proves the daemon wiring end to end: a
// foreign crtx turn published on the daemon bus becomes a readable
// inbox entry under <state>/nerv/inbox, and the daemon's own
// emissions do not.
func TestStartIngest_BusToInbox(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	stop := startIngest(config.IngestConfig{}, b, "me")
	defer stop()

	publishTurn := func(source, envID string) {
		t.Helper()
		if err := b.Publish(context.Background(), bus.NewEvent(
			bus.Topic(crtx.TopicUserReceived), source, map[string]any{
				"envelope_id": envID,
				"turn_id":     "t-1",
				"content":     []any{map[string]any{"type": "text", "text": "hi"}},
			})); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	publishTurn("stem", "foreign-env")
	publishTurn(crtx.BusSource, "own-env")

	store := inbox.NewStore(filepath.Join(state, "nerv", "inbox"))
	entries := waitForEntries(t, store, 1)
	if len(entries) != 1 || entries[0].EnvelopeID != "foreign-env" {
		t.Fatalf("entries = %+v, want exactly the foreign turn", entries)
	}
}

// TestStartIngest_ScannerPicksUpConfiguredRoot proves the scan path:
// a stem-shaped session file in a configured root routes into the
// inbox on the poll tick.
func TestStartIngest_ScannerPicksUpConfiguredRoot(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	root := t.TempDir()
	doc := `{"crtx_version":"0.1","id":"scan-env-1",` +
		`"created_at":"2026-08-08T09:00:00Z","updated_at":"2026-08-08T09:00:00Z",` +
		`"source":{"kind":"stem","version":"0.1.0"},` +
		`"turns":[{"id":"t1","role":"user","created_at":"2026-08-08T09:00:00Z",` +
		`"content":[{"type":"text","text":"scanned"}]}]}` + "\n"
	if err := os.WriteFile(filepath.Join(root, "scan-env-1.jsonl"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	stop := startIngest(config.IngestConfig{
		ScanRoots:           []string{root},
		ScanIntervalSeconds: 1,
	}, b, "")
	defer stop()

	store := inbox.NewStore(filepath.Join(state, "nerv", "inbox"))
	entries := waitForEntries(t, store, 1)
	if entries[0].EnvelopeID != "scan-env-1" || entries[0].Origin.Transport != "scan" {
		t.Fatalf("entries = %+v", entries)
	}
}

// TestStartIngest_DisabledIsInert: ingest.enabled=false must attach
// nothing — foreign traffic on the bus leaves no inbox behind.
func TestStartIngest_DisabledIsInert(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)

	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	off := false
	stop := startIngest(config.IngestConfig{Enabled: &off}, b, "")
	defer stop()

	if err := b.Publish(context.Background(), bus.NewEvent(
		bus.Topic(crtx.TopicUserReceived), "stem", map[string]any{
			"envelope_id": "foreign-env",
			"turn_id":     "t-1",
			"content":     []any{map[string]any{"type": "text", "text": "hi"}},
		})); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(state, "nerv", "inbox")); !os.IsNotExist(err) {
		t.Fatalf("disabled ingest created inbox state (err=%v)", err)
	}
}
