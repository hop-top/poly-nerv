package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSessionFile writes doc as a single compact JSONL line —
// the stem JSONLStore on-disk convention.
func writeSessionFile(t *testing.T, path, doc string) {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(doc)); err != nil {
		t.Fatalf("compact fixture: %v", err)
	}
	buf.WriteByte('\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// specExampleJSONL converts a vendored spec example (pretty-printed
// .json) into the JSONL session-file form producers write.
func specExampleJSONL(t *testing.T, name, dest string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "crtx", "testdata", "examples", name))
	if err != nil {
		t.Fatalf("read spec example %s: %v", name, err)
	}
	writeSessionFile(t, dest, string(raw))
}

func newScanner(t *testing.T, cfg ScannerConfig) (*Scanner, *collector, *Accounts) {
	t.Helper()
	col := &collector{}
	acct := NewAccounts()
	return NewScanner(cfg, col.handle, acct), col, acct
}

// stemSession is a synthetic stem-shaped envelope with one turn of
// every routing class.
const stemSession = `{
  "crtx_version": "0.1",
  "id": "stem-sess-1",
  "created_at": "2026-08-08T09:00:00Z",
  "updated_at": "2026-08-08T09:00:05Z",
  "source": {"kind": "stem", "version": "0.1.0"},
  "turns": [
    {"id": "t1", "role": "user", "created_at": "2026-08-08T09:00:00Z",
     "content": [{"type": "text", "text": "route this"}]},
    {"id": "t2", "role": "assistant", "created_at": "2026-08-08T09:00:01Z",
     "content": [{"type": "text", "text": "an answer"}], "agent_id": "planner"},
    {"id": "t3", "role": "tool", "created_at": "2026-08-08T09:00:02Z",
     "content": [{"type": "tool_result", "call_id": "c1", "output": "ok"}]},
    {"id": "t4", "role": "system", "created_at": "2026-08-08T09:00:03Z",
     "content": [{"type": "text", "text": "plain system prompt"}]},
    {"id": "t5", "role": "developer", "created_at": "2026-08-08T09:00:04Z",
     "content": [{"type": "text", "text": "priority guidance"}]}
  ]
}`

func TestScanner_DeliversRoutableTurnsFromStemShapedFile(t *testing.T) {
	root := t.TempDir()
	writeSessionFile(t, filepath.Join(root, "stem-sess-1.jsonl"), stemSession)

	s, col, acct := newScanner(t, ScannerConfig{Roots: []string{root}})
	s.ScanOnce()

	msgs := col.all()
	if len(msgs) != 3 {
		t.Fatalf("delivered %d messages, want 3 (user, assistant, developer): %+v", len(msgs), msgs)
	}
	byTurn := map[string]Message{}
	for _, m := range msgs {
		byTurn[m.TurnID] = m
	}
	if m, ok := byTurn["t1"]; !ok || m.Role != "user" || m.EnvelopeID != "stem-sess-1" {
		t.Errorf("t1 = %+v", m)
	}
	if m, ok := byTurn["t2"]; !ok || m.Role != "assistant" || m.AgentID != "planner" {
		t.Errorf("t2 = %+v", m)
	}
	if m, ok := byTurn["t5"]; !ok || m.Role != "developer" {
		t.Errorf("t5 = %+v", m)
	}
	for _, m := range msgs {
		if m.Origin.Transport != "scan" || m.Origin.SourceKind != "stem" ||
			m.Origin.SourceVersion != "0.1.0" || m.Origin.Path == "" {
			t.Errorf("origin = %+v", m.Origin)
		}
	}
	snap := acct.Snapshot()
	if snap["accepted"] != 3 || snap["skipped"] != 2 {
		t.Errorf("accepted=%d skipped=%d, want 3/2", snap["accepted"], snap["skipped"])
	}
}

func TestScanner_SpecExampleFixtureRoutes(t *testing.T) {
	root := t.TempDir()
	specExampleJSONL(t, "tool-call.json", filepath.Join(root, "01JCRTX0EXAMPLETOOLCALL0001.jsonl"))

	s, col, _ := newScanner(t, ScannerConfig{Roots: []string{root}})
	s.ScanOnce()

	// user + two assistant turns route; the tool turn is mechanics.
	msgs := col.all()
	if len(msgs) != 3 {
		t.Fatalf("delivered %d, want 3: %+v", len(msgs), msgs)
	}
}

func TestScanner_MidFlightSystemTurnRoutes(t *testing.T) {
	root := t.TempDir()
	writeSessionFile(t, filepath.Join(root, "mid.jsonl"), `{
	  "crtx_version": "0.1", "id": "mid-1",
	  "created_at": "2026-08-08T09:00:00Z", "updated_at": "2026-08-08T09:00:02Z",
	  "source": {"kind": "custom", "version": "1.0.0"},
	  "turns": [
	    {"id": "t1", "role": "assistant", "created_at": "2026-08-08T09:00:00Z",
	     "content": [{"type": "tool_call", "call_id": "c7", "name": "worker",
	                  "input": {"job": "long"}}]},
	    {"id": "t2", "role": "system", "created_at": "2026-08-08T09:00:01Z",
	     "in_reply_to_call_id": "c7",
	     "content": [{"type": "text", "text": "abort the job"}]}
	  ]
	}`)

	s, col, _ := newScanner(t, ScannerConfig{Roots: []string{root}})
	s.ScanOnce()

	msgs := col.all()
	if len(msgs) != 2 {
		t.Fatalf("delivered %d, want 2: %+v", len(msgs), msgs)
	}
	var sys *Message
	for i := range msgs {
		if msgs[i].Role == "system" {
			sys = &msgs[i]
		}
	}
	if sys == nil || sys.InReplyToCallID != "c7" {
		t.Fatalf("mid-flight system turn not routed: %+v", msgs)
	}
}

func TestScanner_VersionGate(t *testing.T) {
	root := t.TempDir()
	writeSessionFile(t, filepath.Join(root, "future.jsonl"), `{
	  "crtx_version": "0.2", "id": "future-1",
	  "created_at": "2026-08-08T09:00:00Z", "updated_at": "2026-08-08T09:00:00Z",
	  "source": {"kind": "stem", "version": "9.9.9"},
	  "turns": [{"id": "t1", "role": "user",
	    "created_at": "2026-08-08T09:00:00Z",
	    "content": [{"type": "text", "text": "from the future"}]}]
	}`)

	s, col, acct := newScanner(t, ScannerConfig{Roots: []string{root}})
	s.ScanOnce()

	if got := col.all(); len(got) != 0 {
		t.Fatalf("future-version envelope delivered: %+v", got)
	}
	if got := acct.Snapshot()["version"]; got != 1 {
		t.Errorf("version = %d, want 1", got)
	}
}

func TestScanner_InvalidFilesDroppedWithAccount(t *testing.T) {
	root := t.TempDir()
	// Not JSON at all.
	if err := os.WriteFile(filepath.Join(root, "torn.jsonl"),
		[]byte(`{"crtx_version":"0.1","id":"torn-1","turns":[{`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Schema-invalid: role outside the v0.1 enum rejects the whole
	// envelope — no partial delivery of the valid first turn.
	writeSessionFile(t, filepath.Join(root, "badrole.jsonl"), `{
	  "crtx_version": "0.1", "id": "badrole-1",
	  "created_at": "2026-08-08T09:00:00Z", "updated_at": "2026-08-08T09:00:01Z",
	  "source": {"kind": "stem", "version": "0.1.0"},
	  "turns": [
	    {"id": "t1", "role": "user", "created_at": "2026-08-08T09:00:00Z",
	     "content": [{"type": "text", "text": "fine"}]},
	    {"id": "t2", "role": "robot", "created_at": "2026-08-08T09:00:01Z",
	     "content": [{"type": "text", "text": "not fine"}]}
	  ]
	}`)
	// Non-jsonl files are not session files.
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, col, acct := newScanner(t, ScannerConfig{Roots: []string{root}})
	s.ScanOnce()

	if got := col.all(); len(got) != 0 {
		t.Fatalf("invalid files delivered: %+v", got)
	}
	if got := acct.Snapshot()["invalid"]; got != 2 {
		t.Errorf("invalid = %d, want 2", got)
	}
}

func TestScanner_ChangeDriven(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "grow.jsonl")
	writeSessionFile(t, path, `{
	  "crtx_version": "0.1", "id": "grow-1",
	  "created_at": "2026-08-08T09:00:00Z", "updated_at": "2026-08-08T09:00:00Z",
	  "source": {"kind": "stem", "version": "0.1.0"},
	  "turns": [{"id": "t1", "role": "user",
	    "created_at": "2026-08-08T09:00:00Z",
	    "content": [{"type": "text", "text": "first"}]}]
	}`)

	s, col, _ := newScanner(t, ScannerConfig{Roots: []string{root}})
	s.ScanOnce()
	if got := len(col.all()); got != 1 {
		t.Fatalf("first pass delivered %d, want 1", got)
	}

	// Unchanged file: second pass must not re-read / re-deliver.
	s.ScanOnce()
	if got := len(col.all()); got != 1 {
		t.Fatalf("unchanged file re-delivered: %d messages", got)
	}

	// Producer appends a turn (whole-file rewrite, stem convention).
	writeSessionFile(t, path, `{
	  "crtx_version": "0.1", "id": "grow-1",
	  "created_at": "2026-08-08T09:00:00Z", "updated_at": "2026-08-08T09:00:05Z",
	  "source": {"kind": "stem", "version": "0.1.0"},
	  "turns": [
	    {"id": "t1", "role": "user", "created_at": "2026-08-08T09:00:00Z",
	     "content": [{"type": "text", "text": "first"}]},
	    {"id": "t2", "role": "assistant", "created_at": "2026-08-08T09:00:05Z",
	     "content": [{"type": "text", "text": "second"}]}
	  ]
	}`)

	// Changed file re-delivers every routable turn; downstream dedup
	// (msg id = envelope+turn digest) makes the overlap a no-op.
	s.ScanOnce()
	if got := len(col.all()); got != 3 {
		t.Fatalf("changed file pass delivered %d total, want 3", got)
	}
}

func TestScanner_WalksOneNestedLevel(t *testing.T) {
	root := t.TempDir()
	writeSessionFile(t, filepath.Join(root, "agent-a", "nested-1.jsonl"), `{
	  "crtx_version": "0.1", "id": "nested-1",
	  "created_at": "2026-08-08T09:00:00Z", "updated_at": "2026-08-08T09:00:00Z",
	  "source": {"kind": "stem", "version": "0.1.0"},
	  "turns": [{"id": "t1", "role": "user",
	    "created_at": "2026-08-08T09:00:00Z",
	    "content": [{"type": "text", "text": "nested"}]}]
	}`)

	s, col, _ := newScanner(t, ScannerConfig{Roots: []string{root}})
	s.ScanOnce()
	if got := len(col.all()); got != 1 {
		t.Fatalf("nested file delivered %d, want 1", got)
	}
}

func TestScanner_RejectsRootsInsideOwnStateDir(t *testing.T) {
	state := t.TempDir()
	own := filepath.Join(state, "nerv", "sessions")
	writeSessionFile(t, filepath.Join(own, "self-1.jsonl"), stemSession)

	s, col, _ := newScanner(t, ScannerConfig{
		Roots:       []string{own},
		ExcludeRoot: filepath.Join(state, "nerv"),
	})
	s.ScanOnce()

	if got := col.all(); len(got) != 0 {
		t.Fatalf("own state dir was scanned: %+v", got)
	}
}

// TestScanner_RunDeliversOnTick covers the daemon loop under -race:
// Run polls in the background and picks up files that appear later.
func TestScanner_RunDeliversOnTick(t *testing.T) {
	root := t.TempDir()
	s, col, _ := newScanner(t, ScannerConfig{
		Roots:    []string{root},
		Interval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()

	writeSessionFile(t, filepath.Join(root, "late.jsonl"), stemSession)

	deadline := time.After(3 * time.Second)
	for len(col.all()) < 3 {
		select {
		case <-deadline:
			cancel()
			<-done
			t.Fatalf("timed out; delivered %d", len(col.all()))
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done
}
