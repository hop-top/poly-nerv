package crtx

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/envelope"
)

// publishAll runs every hook envelope through Translate and publishes
// the emissions — the exact traffic the dispatcher generates.
func publishAll(t *testing.T, b bus.Bus, envs ...*envelope.Envelope) {
	t.Helper()
	for _, env := range envs {
		for _, em := range Translate(env) {
			e := bus.NewEvent(bus.Topic(em.Topic), BusSource, em.Payload)
			if err := b.Publish(context.Background(), e); err != nil {
				t.Fatalf("publish %s: %v", em.Topic, err)
			}
		}
	}
}

// TestRecorder_EndToEnd drives a realistic hook sequence through
// translator → bus → recorder and checks the persisted envelope is a
// schema-valid crtx v0.1 conversation.
func TestRecorder_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	rec := NewRecorder(store)
	unsub := rec.Attach(b)
	defer unsub()

	publishAll(t, b,
		hookEnv("SessionStart", "ev-1", `{"session_id":"9f36c8a2-6c5f-4a51-b2f4-1f2f3a4b5c6d"}`),
		hookEnv("UserPromptSubmit", "ev-2", `{"prompt":"list files"}`),
		hookEnv("PreToolUse", "ev-3", `{"tool_name":"Bash","tool_input":{"command":"ls"}}`),
		hookEnv("PostToolUse", "ev-4", `{"tool_name":"Bash","tool_input":{"command":"ls"},"tool_response":{"stdout":"main.go"}}`),
		hookEnv("Stop", "ev-5", `{"stop_reason":"end_turn","turn_index":1}`),
		hookEnv("FileChanged", "ev-6", `{"file_path":"/tmp/x"}`), // dropped: must not appear
		hookEnv("SessionEnd", "ev-7", `{"session_id":"9f36c8a2-6c5f-4a51-b2f4-1f2f3a4b5c6d","exit_code":0,"duration_ms":1200}`),
	)

	path := filepath.Join(dir, "9f36c8a2-6c5f-4a51-b2f4-1f2f3a4b5c6d.jsonl")
	env, line := readSessionFile(t, path)
	validateEnvelopeBytes(t, line)

	if env.CrtxVersion != Version {
		t.Errorf("crtx_version = %q", env.CrtxVersion)
	}
	if env.Source.Kind != "claude-code" {
		t.Errorf("source.kind = %q, want claude-code", env.Source.Kind)
	}

	wantRoles := []string{"user", "assistant", "tool", "assistant"}
	if len(env.Turns) != len(wantRoles) {
		t.Fatalf("turns = %d, want %d: %s", len(env.Turns), len(wantRoles), line)
	}
	for i, want := range wantRoles {
		if env.Turns[i].Role != want {
			t.Errorf("turns[%d].role = %q, want %q", i, env.Turns[i].Role, want)
		}
	}

	// tool_call / tool_result parts must pair on call_id.
	var call struct {
		Type   string `json:"type"`
		CallID string `json:"call_id"`
	}
	var result struct {
		Type   string `json:"type"`
		CallID string `json:"call_id"`
	}
	if err := json.Unmarshal(env.Turns[1].Content[0], &call); err != nil {
		t.Fatalf("decode tool_call part: %v", err)
	}
	if err := json.Unmarshal(env.Turns[2].Content[0], &result); err != nil {
		t.Fatalf("decode tool_result part: %v", err)
	}
	if call.Type != "tool_call" || result.Type != "tool_result" {
		t.Errorf("part types = %q,%q", call.Type, result.Type)
	}
	if call.CallID == "" || call.CallID != result.CallID {
		t.Errorf("call ids do not pair: %q vs %q", call.CallID, result.CallID)
	}

	if env.Metadata[MetaSessionEnd] == nil {
		t.Errorf("metadata[%s] missing after SessionEnd", MetaSessionEnd)
	}
	if env.Metadata[MetaSynthesized] != nil {
		t.Errorf("synthesized flag set despite explicit SessionStart")
	}
}

// TestRecorder_AutoCreatesForLateJoin verifies a turn event for an
// unseen envelope synthesizes a schema-valid skeleton.
func TestRecorder_AutoCreatesForLateJoin(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	rec := NewRecorder(store)
	defer rec.Attach(b)()

	publishAll(t, b, hookEnv("UserPromptSubmit", "ev-1", `{"prompt":"resumed mid-session"}`))

	env, line := readSessionFile(t, filepath.Join(dir, "9f36c8a2-6c5f-4a51-b2f4-1f2f3a4b5c6d.jsonl"))
	validateEnvelopeBytes(t, line)
	if env.Metadata[MetaSynthesized] != true {
		t.Errorf("metadata[%s] = %v, want true", MetaSynthesized, env.Metadata[MetaSynthesized])
	}
	if len(env.Turns) != 1 || env.Turns[0].Role != "user" {
		t.Errorf("turns = %+v, want one user turn", env.Turns)
	}
}

// TestRecorder_HandlesCrossProcessPayloadForm verifies the recorder
// accepts JSON-decoded payloads (map form) the way network/sqlite
// adapters deliver them, not just in-process typed structs.
func TestRecorder_HandlesCrossProcessPayloadForm(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	rec := NewRecorder(store)
	defer rec.Attach(b)()

	payload := map[string]any{
		"envelope_id": "sess-x",
		"turn_id":     "t-1",
		"content":     []any{map[string]any{"type": "text", "text": "over the wire"}},
	}
	if err := b.Publish(context.Background(),
		bus.NewEvent(bus.Topic(TopicUserReceived), BusSource, payload)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	env, line := readSessionFile(t, filepath.Join(dir, "sess-x.jsonl"))
	validateEnvelopeBytes(t, line)
	if len(env.Turns) != 1 || env.Turns[0].ID != "t-1" {
		t.Fatalf("turns = %+v, want the wire-form turn", env.Turns)
	}
}

// TestRecorder_MalformedPayloadFailOpen verifies junk on a canonical
// topic is logged and skipped without failing the publisher.
func TestRecorder_MalformedPayloadFailOpen(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	rec := NewRecorder(store)
	defer rec.Attach(b)()

	if err := b.Publish(context.Background(),
		bus.NewEvent(bus.Topic(TopicUserReceived), BusSource, "not a payload")); err != nil {
		t.Fatalf("publish must not fail on malformed payload: %v", err)
	}
}

// TestRecorder_IgnoresForeignSourceEvents locks in the consumer-ADR
// scope narrowing: the recorder folds only this producer's own
// emissions (Event.Source == BusSource). A foreign producer's crtx
// events arriving on the same bus (network adapter fan-in) must not
// materialize envelope files under the local sessions scan root —
// that transcript belongs to the foreign producer's own store.
func TestRecorder_IgnoresForeignSourceEvents(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	b := bus.New()
	defer func() { _ = b.Close(context.Background()) }()

	rec := NewRecorder(store)
	defer rec.Attach(b)()

	foreign := bus.NewEvent(bus.Topic(TopicSessionCreated), "stem", SessionCreatedPayload{
		EnvelopeID: "foreign-env-1",
		CreatedAt:  "2026-08-08T10:00:00Z",
		Source:     Source{Kind: "stem", Version: "0.1.0"},
	})
	if err := b.Publish(context.Background(), foreign); err != nil {
		t.Fatalf("publish foreign created: %v", err)
	}
	foreignTurn := bus.NewEvent(bus.Topic(TopicUserReceived), "stem", UserReceivedPayload{
		EnvelopeID: "foreign-env-1",
		TurnID:     "ft-1",
		Content:    []any{NewTextPart("hello from another agent")},
	})
	if err := b.Publish(context.Background(), foreignTurn); err != nil {
		t.Fatalf("publish foreign turn: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "foreign-env-1.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("foreign-source event materialized a local session file (err=%v)", err)
	}

	// Own emissions still fold (source == BusSource).
	own := bus.NewEvent(bus.Topic(TopicSessionCreated), BusSource, SessionCreatedPayload{
		EnvelopeID: "own-env-1",
		CreatedAt:  "2026-08-08T10:00:00Z",
		Source:     Source{Kind: "claude-code", Version: "2.1.0"},
	})
	if err := b.Publish(context.Background(), own); err != nil {
		t.Fatalf("publish own created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "own-env-1.jsonl")); err != nil {
		t.Fatalf("own-source event did not materialize session file: %v", err)
	}
}
