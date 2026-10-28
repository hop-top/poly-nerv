package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testEntry(msgID string, at time.Time) Entry {
	return Entry{
		SchemaVersion: EntrySchemaVersion,
		MsgID:         msgID,
		InboxID:       InboxForeignTurns,
		Kind:          KindForeignTurn,
		ReceivedAt:    at,
		EnvelopeID:    "env-1",
		TurnID:        "t-" + msgID,
		Role:          "user",
		Content:       []json.RawMessage{json.RawMessage(`{"type":"text","text":"hi"}`)},
		Preview:       "hi",
	}
}

func TestStore_PutListGet(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	t0 := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	for i, id := range []string{"m1", "m2", "m3"} {
		delivered, err := s.Put(testEntry(id, t0.Add(time.Duration(i)*time.Minute)))
		if err != nil {
			t.Fatalf("Put %s: %v", id, err)
		}
		if !delivered {
			t.Fatalf("Put %s reported duplicate on first delivery", id)
		}
	}

	// One JSON file per message in the container directory.
	files, err := filepath.Glob(filepath.Join(dir, InboxForeignTurns, "*.json"))
	if err != nil || len(files) != 3 {
		t.Fatalf("container files = %v (err=%v), want 3", files, err)
	}

	// List: newest first.
	entries, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("List len = %d, want 3", len(entries))
	}
	if entries[0].MsgID != "m3" || entries[2].MsgID != "m1" {
		t.Errorf("List order = %s,%s,%s; want newest first",
			entries[0].MsgID, entries[1].MsgID, entries[2].MsgID)
	}

	got, err := s.Get("m2")
	if err != nil {
		t.Fatalf("Get m2: %v", err)
	}
	if got.TurnID != "t-m2" || got.Role != "user" {
		t.Errorf("Get m2 = %+v", got)
	}
	var part struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(got.Content[0], &part); err != nil || part.Text != "hi" {
		t.Errorf("content not preserved: %s", got.Content[0])
	}

	if _, err := s.Get("missing"); err == nil {
		t.Error("Get missing returned nil error")
	}
}

func TestStore_PutDuplicateIsNoOp(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	e := testEntry("m1", time.Now().UTC())
	if delivered, err := s.Put(e); err != nil || !delivered {
		t.Fatalf("first Put = %v/%v", delivered, err)
	}

	// Same msg id with different content must not overwrite.
	e2 := e
	e2.Preview = "changed"
	delivered, err := s.Put(e2)
	if err != nil {
		t.Fatalf("duplicate Put: %v", err)
	}
	if delivered {
		t.Fatal("duplicate Put reported delivery")
	}
	got, err := s.Get("m1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Preview != "hi" {
		t.Errorf("duplicate overwrote entry: preview = %q", got.Preview)
	}
}

func TestStore_RejectsUnsafeIDs(t *testing.T) {
	s := NewStore(t.TempDir())
	for _, id := range []string{"", ".", "..", "a/b", `a\b`, "../escape"} {
		e := testEntry("m1", time.Now().UTC())
		e.MsgID = id
		if _, err := s.Put(e); err == nil {
			t.Errorf("msg id %q accepted", id)
		}
	}
	e := testEntry("m1", time.Now().UTC())
	e.InboxID = "../outside"
	if _, err := s.Put(e); err == nil {
		t.Error("unsafe inbox id accepted")
	}
}

func TestStore_ListSkipsForeignFiles(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if _, err := s.Put(testEntry("m1", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	// Non-JSON junk and stray files must not break listing.
	if err := os.WriteFile(filepath.Join(dir, InboxForeignTurns, "junk.json"),
		[]byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].MsgID != "m1" {
		t.Errorf("List = %+v, want just m1", entries)
	}
}
