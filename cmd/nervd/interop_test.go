package main

// Interop: end-to-end route-through with a second agent's stem-shaped
// session, plus the indexer-side hygiene invariant on this producer's
// own scan root.
//
// Coverage limits (the upstream crtx conformance suite is planned but
// unpublished; until it ships, these tests are the enforceable proxy):
//
//   - Foreign fixtures are stem-SHAPED: byte conventions replicated
//     from the stem JSONLStore contract (one compact envelope line per
//     file, trailing newline, atomic whole-file rewrite), validated
//     against the vendored v0.1 schemas — not produced by a live stem
//     build. A stem release that drifts from its own conventions would
//     not be caught here.
//   - The indexer mirror below replicates the published reader
//     discipline (one-level walk, last-line read, crtx_version gate,
//     silent skip); it is not the indexer's own code.
//   - Remote nerv-to-nerv bus traffic is out of coverage by decision:
//     the source-based self-origin filter drops it (consumer ADR
//     §"Self-origin filtering").
//   - Cross-process concurrency (a real stem writing while this
//     daemon scans) is approximated by whole-file rewrites between
//     scan passes; true multi-process interleaving needs the
//     conformance harness.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/crtx"
	"hop.top/nerv/internal/dispatch"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/eventlog"
	"hop.top/nerv/internal/inbox"
	"hop.top/nerv/internal/ingest"
)

// ── indexer-side reader mirror (vein discipline) ────────────────────

func indexerScan(root string) []string {
	top, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range top {
		full := filepath.Join(root, e.Name())
		if e.IsDir() {
			inner, err := os.ReadDir(full)
			if err != nil {
				continue
			}
			for _, ie := range inner {
				if !ie.IsDir() && strings.HasSuffix(ie.Name(), ".jsonl") {
					out = append(out, filepath.Join(full, ie.Name()))
				}
			}
			continue
		}
		if strings.HasSuffix(e.Name(), ".jsonl") {
			out = append(out, full)
		}
	}
	return out
}

func indexerLastLine(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("indexer open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	var last []byte
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if trimmed := bytes.TrimRight(line, "\r\n"); len(trimmed) > 0 {
			last = append(last[:0], trimmed...)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("indexer read %s: %v", path, err)
		}
	}
	return last
}

// ── driving the local producer (this tool's own hook stream) ────────

func hookEnvAt(event, eventID, sessionID, payload string) *envelope.Envelope {
	return &envelope.Envelope{
		NervVersion: "1.0",
		Event:       event,
		EventID:     eventID,
		Timestamp:   time.Now(),
		CLI:         envelope.CLIInfo{Name: "claude", Version: "2.1.0"},
		Session:     envelope.SessionInfo{ID: sessionID},
		Payload:     json.RawMessage(payload),
	}
}

func driveOwnSession(t *testing.T, d *dispatch.Dispatcher, sessionID string) {
	t.Helper()
	for _, env := range []*envelope.Envelope{
		hookEnvAt("SessionStart", "ev-1", sessionID, `{"session_id":"`+sessionID+`"}`),
		hookEnvAt("UserPromptSubmit", "ev-2", sessionID, `{"prompt":"summarize the report"}`),
		hookEnvAt("PreToolUse", "ev-3", sessionID, `{"tool_name":"Bash","tool_input":{"command":"wc -l report.md"}}`),
		hookEnvAt("PostToolUse", "ev-4", sessionID, `{"tool_name":"Bash","tool_input":{"command":"wc -l report.md"},"tool_response":{"stdout":"142"}}`),
		hookEnvAt("Stop", "ev-5", sessionID, `{"stop_reason":"end_turn","turn_index":1}`),
		hookEnvAt("SessionEnd", "ev-6", sessionID, `{"session_id":"`+sessionID+`","exit_code":0}`),
	} {
		if _, err := d.Dispatch(env); err != nil {
			t.Fatalf("dispatch %s: %v", env.Event, err)
		}
	}
}

// stemSessionDoc returns a stem-shaped envelope: dispatched from the
// local session (lineage correlation) with one user and one assistant
// turn.
func stemSessionDoc(localSessionID string) string {
	return `{
	  "crtx_version": "0.1",
	  "id": "stem-agent-sess-1",
	  "created_at": "2026-08-08T09:00:00Z",
	  "updated_at": "2026-08-08T09:00:02Z",
	  "source": {"kind": "stem", "version": "0.1.0", "instance": "agent-b/pid-99"},
	  "dispatched_from": {"envelope_id": "` + localSessionID + `", "call_id": "call-42"},
	  "turns": [
	    {"id": "st-1", "role": "user", "created_at": "2026-08-08T09:00:01Z",
	     "content": [{"type": "text", "text": "status update please"}]},
	    {"id": "st-2", "role": "assistant", "created_at": "2026-08-08T09:00:02Z",
	     "content": [{"type": "text", "text": "done: 3 of 4 subtasks complete"}],
	     "agent_id": "agent-b"}
	  ]
	}`
}

// writeStemConvention writes doc using the stem JSONLStore byte
// conventions: one compact JSON line, trailing newline, whole-file
// rename into place.
func writeStemConvention(t *testing.T, path, doc string) {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(doc)); err != nil {
		t.Fatalf("compact stem doc: %v", err)
	}
	buf.WriteByte('\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

// TestInterop_StemSessionRoutesThroughToInbox is the headline scenario:
// with the full daemon wiring live (producer recorder + consume side),
// a second agent's stem-shaped session — discovered at the default
// scan root — and a live stem bus event both land in the inbox, with
// attribution and lineage correlation, exactly once each even after
// replay.
func TestInterop_StemSessionRoutesThroughToInbox(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	const ownSession = "9a1b2c3d-0000-4000-8000-interop00001"

	// Daemon wiring: bus, producer-side recorder, real eventlog,
	// dispatcher, consume side (default scan roots — no config).
	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()
	defer attachSessionRecorder(b)()

	d := dispatch.New(
		config.Compile(&config.HooksConfig{}),
		&eventlog.JSONLLogger{},
		dispatch.NewMatcher(),
		dispatch.NewRunnerWithBus(b),
	)

	// The local producer runs first so the stem session's
	// dispatched_from target exists for correlation.
	driveOwnSession(t, d, ownSession)

	// Second agent: a stem-shaped session at stem's convention path,
	// discovered via the default scan root (no ingest config).
	stemSessions := filepath.Join(state, "stem", "sessions")
	writeStemConvention(t, filepath.Join(stemSessions, "stem-agent-sess-1.jsonl"),
		stemSessionDoc(ownSession))

	stop := startIngest(config.IngestConfig{ScanIntervalSeconds: 1}, b, "interop-agent")
	defer stop()

	// Second agent, live path: a stem event over the bus.
	livePayload := map[string]any{
		"envelope_id": "stem-live-sess",
		"turn_id":     "lt-1",
		"content":     []any{map[string]any{"type": "text", "text": "live ping"}},
	}
	if err := b.Publish(context.Background(),
		bus.NewEvent(bus.Topic(crtx.TopicUserReceived), "stem", livePayload)); err != nil {
		t.Fatalf("live publish: %v", err)
	}

	store := inbox.NewStore(filepath.Join(state, "nerv", "inbox"))
	waitForEntries(t, store, 3)

	// Replay both paths: same bus event again; stem file rewritten
	// (mtime change forces a re-read). Dedup must hold the line.
	if err := b.Publish(context.Background(),
		bus.NewEvent(bus.Topic(crtx.TopicUserReceived), "stem", livePayload)); err != nil {
		t.Fatalf("replay publish: %v", err)
	}
	writeStemConvention(t, filepath.Join(stemSessions, "stem-agent-sess-1.jsonl"),
		stemSessionDoc(ownSession))
	time.Sleep(1500 * time.Millisecond) // ≥ one scan tick
	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("after replay: %d entries, want 3 (dedup)", len(entries))
	}

	byID := map[string]inbox.Entry{}
	for _, e := range entries {
		byID[e.MsgID] = e
	}

	// Scan-path user turn: attribution + lineage correlation.
	scanUser, ok := byID[inbox.MessageID("stem-agent-sess-1", "st-1")]
	if !ok {
		t.Fatalf("stem user turn missing from inbox: %+v", entries)
	}
	if scanUser.Origin.Transport != "scan" || scanUser.Origin.SourceKind != "stem" {
		t.Errorf("scan attribution = %+v", scanUser.Origin)
	}
	if scanUser.Correlation == nil ||
		scanUser.Correlation.LocalSessionID != ownSession ||
		scanUser.Correlation.Relation != "dispatched_from" {
		t.Errorf("lineage correlation = %+v", scanUser.Correlation)
	}
	if scanUser.Preview != "status update please" {
		t.Errorf("preview = %q", scanUser.Preview)
	}

	// Scan-path assistant turn keeps agent attribution.
	scanAsst, ok := byID[inbox.MessageID("stem-agent-sess-1", "st-2")]
	if !ok || scanAsst.AgentID != "agent-b" || scanAsst.Role != "assistant" {
		t.Errorf("stem assistant turn = %+v (ok=%v)", scanAsst, ok)
	}

	// Bus-path turn: live attribution.
	live, ok := byID[inbox.MessageID("stem-live-sess", "lt-1")]
	if !ok || live.Origin.Transport != "bus" || live.Origin.BusSource != "stem" {
		t.Errorf("live turn = %+v (ok=%v)", live, ok)
	}
}

// TestInterop_OwnScanRootStaysClean is the indexer-side invariant: an
// indexer walking this producer's sessions/ scan root — with the
// published reader discipline — finds exactly the producer's own
// schema-valid envelopes. Internal logs (eventlog, bus artifacts),
// inbox entries, and foreign sessions never leak in, even while the
// consume side is live and routing foreign traffic.
func TestInterop_OwnScanRootStaysClean(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	const ownSession = "9a1b2c3d-0000-4000-8000-interop00002"

	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()
	defer attachSessionRecorder(b)()

	stemSessions := filepath.Join(state, "stem", "sessions")
	writeStemConvention(t, filepath.Join(stemSessions, "stem-agent-sess-1.jsonl"),
		stemSessionDoc(ownSession))
	stop := startIngest(config.IngestConfig{ScanIntervalSeconds: 1}, b, "")
	defer stop()

	d := dispatch.New(
		config.Compile(&config.HooksConfig{}),
		&eventlog.JSONLLogger{},
		dispatch.NewMatcher(),
		dispatch.NewRunnerWithBus(b),
	)
	driveOwnSession(t, d, ownSession)

	// Foreign traffic on the bus while the recorder is attached: the
	// narrowed recorder must not materialize it under sessions/.
	if err := b.Publish(context.Background(), bus.NewEvent(
		bus.Topic(crtx.TopicSessionCreated), "stem", map[string]any{
			"envelope_id": "foreign-created",
			"created_at":  "2026-08-08T09:00:00Z",
			"source":      map[string]any{"kind": "stem", "version": "0.1.0"},
		})); err != nil {
		t.Fatal(err)
	}

	// Wait until the consume side has routed the stem session, so the
	// hygiene check runs with the whole pipeline warm.
	store := inbox.NewStore(filepath.Join(state, "nerv", "inbox"))
	waitForEntries(t, store, 2)

	// Internal surfaces exist...
	if files, _ := filepath.Glob(filepath.Join(state, "nerv", "events", "*.jsonl")); len(files) == 0 {
		t.Fatal("eventlog never materialized; hygiene check would be vacuous")
	}

	// ...and the scan root holds exactly the producer's own envelope.
	sessions := filepath.Join(state, "nerv", "sessions")
	found := indexerScan(sessions)
	if len(found) != 1 {
		t.Fatalf("indexer found %d files under sessions/, want 1: %v", len(found), found)
	}
	if filepath.Base(found[0]) != ownSession+".jsonl" {
		t.Errorf("unexpected session file: %s", found[0])
	}

	line := indexerLastLine(t, found[0])
	if err := crtx.ValidateEnvelopeBytes(line); err != nil {
		t.Errorf("own envelope fails v0.1 schema (indexer would skip us): %v", err)
	}
	var env crtx.Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		t.Fatalf("own envelope unparseable: %v", err)
	}
	if env.ID != ownSession || env.Source.Kind != "claude-code" {
		t.Errorf("own envelope identity = %s / %s", env.ID, env.Source.Kind)
	}
	if len(env.Turns) == 0 {
		t.Error("own envelope has no turns")
	}
	if env.Metadata[crtx.MetaSynthesized] == true {
		t.Error("own envelope marked synthesized; recorder missed the created event")
	}

	// The inbox delivery surface holds entry documents, not envelope
	// JSONL — an indexer pointed at it by mistake finds nothing.
	if leaked := indexerScan(filepath.Join(state, "nerv", "inbox")); len(leaked) != 0 {
		t.Errorf("inbox subtree contains envelope-shaped files: %v", leaked)
	}
}

// TestInterop_OwnOutputRoundTripsAsForeignInput closes the loop: an
// envelope this producer wrote is byte-for-byte valid foreign input —
// another agent running the same stack would route it. Guards the
// producer/consumer contract against drifting apart.
func TestInterop_OwnOutputRoundTripsAsForeignInput(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	const ownSession = "9a1b2c3d-0000-4000-8000-interop00003"

	// Produce a real session.
	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()
	defer attachSessionRecorder(b)()
	d := dispatch.New(config.Compile(&config.HooksConfig{}), nil,
		dispatch.NewMatcher(), dispatch.NewRunnerWithBus(b))
	driveOwnSession(t, d, ownSession)

	produced := filepath.Join(state, "nerv", "sessions", ownSession+".jsonl")
	raw, err := os.ReadFile(produced)
	if err != nil {
		t.Fatalf("own session missing: %v", err)
	}

	// Feed the exact bytes to a second consume stack as foreign input.
	foreignRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(foreignRoot, ownSession+".jsonl"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	store := inbox.NewStore(t.TempDir())
	router := inbox.NewRouter(store, inbox.RouterConfig{})
	acct := ingest.NewAccounts()
	scanner := ingest.NewScanner(ingest.ScannerConfig{Roots: []string{foreignRoot}},
		router.Handle, acct)
	scanner.ScanOnce()

	entries, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	// driveOwnSession yields three message-like turns: the user
	// prompt, the tool_call-bearing assistant turn (calls are issued
	// by the assistant, envelope spec §6.2; the role gate routes all
	// assistant turns), and the stop-extension assistant turn. The
	// tool-role result turn is mechanics and stays out.
	if len(entries) != 3 {
		t.Fatalf("round-trip delivered %d entries, want 3; accounts=%v",
			len(entries), acct.Snapshot())
	}
	roles := map[string]bool{}
	for _, e := range entries {
		if e.EnvelopeID != ownSession {
			t.Errorf("entry envelope = %q", e.EnvelopeID)
		}
		roles[e.Role] = true
	}
	if !roles["user"] || !roles["assistant"] {
		t.Errorf("round-trip roles = %v, want user+assistant", roles)
	}
	if snap := acct.Snapshot(); snap["invalid"] != 0 || snap["version"] != 0 {
		t.Errorf("own output rejected as foreign input: %v", snap)
	}
}
