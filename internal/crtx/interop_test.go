package crtx

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

	"github.com/santhosh-tekuri/jsonschema/v6"
	kitbus "hop.top/kit/go/runtime/bus"
)

// ── consumer-side reader mirror ─────────────────────────────────────
//
// The downstream indexer (vein) is a pure reader that deliberately
// re-declares the crtx type surface instead of importing any
// producer's library; its adapter reads the LAST non-empty line of
// each *.jsonl file, gates on crtx_version, walks scan roots one
// directory level deep, and silently skips files that fail to parse.
// The types + read discipline below mirror that consumer contract so
// interop breaks fail here, in the producer's suite.

type consumerEnvelope struct {
	CrtxVersion    string                  `json:"crtx_version"`
	ID             string                  `json:"id"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
	Source         consumerSource          `json:"source"`
	Turns          []consumerTurn          `json:"turns"`
	ParentID       string                  `json:"parent_id,omitempty"`
	ForkPoint      *int                    `json:"fork_point,omitempty"`
	DispatchedFrom *consumerDispatchedFrom `json:"dispatched_from,omitempty"`
	InjectedTurns  []consumerInjectedTurn  `json:"injected_turns,omitempty"`
	Metadata       map[string]any          `json:"metadata,omitempty"`
}

type consumerSource struct {
	Kind     string `json:"kind"`
	Version  string `json:"version"`
	Instance string `json:"instance,omitempty"`
}

type consumerDispatchedFrom struct {
	EnvelopeID string `json:"envelope_id"`
	CallID     string `json:"call_id"`
}

type consumerInjectedTurn struct {
	EnvelopeID          string `json:"envelope_id"`
	StartTurnID         string `json:"start_turn_id"`
	EndTurnID           string `json:"end_turn_id"`
	InjectedAfterTurnID string `json:"injected_after_turn_id,omitempty"`
}

type consumerTurn struct {
	ID              string            `json:"id"`
	Role            string            `json:"role"`
	CreatedAt       time.Time         `json:"created_at"`
	Content         []json.RawMessage `json:"content"`
	AgentID         string            `json:"agent_id,omitempty"`
	InReplyToCallID string            `json:"in_reply_to_call_id,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
}

const consumerSupportedVersion = "0.1"

// consumerReadEnvelopeFile mirrors the consumer's last-line read.
func consumerReadEnvelopeFile(path string) (*consumerEnvelope, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var last []byte
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		trimmed := bytes.TrimRight(line, "\r\n")
		if len(trimmed) > 0 {
			last = append(last[:0], trimmed...)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
	}
	if len(last) == 0 {
		return nil, errors.New("empty envelope file")
	}
	var env consumerEnvelope
	if err := json.Unmarshal(last, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// consumerScanDir mirrors the consumer's directory walk: *.jsonl one
// level deep, plus one nested directory level.
func consumerScanDir(root string) ([]string, error) {
	top, err := os.ReadDir(root)
	if err != nil {
		return nil, err
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
				if ie.IsDir() || !strings.HasSuffix(ie.Name(), ".jsonl") {
					continue
				}
				out = append(out, filepath.Join(full, ie.Name()))
			}
			continue
		}
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		out = append(out, full)
	}
	return out, nil
}

// consumerPartKind mirrors the consumer's content-part discriminator
// probe.
func consumerPartKind(raw json.RawMessage) string {
	var disc struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &disc); err != nil {
		return ""
	}
	return disc.Type
}

// ── spec example fixtures ───────────────────────────────────────────

// TestSpecEnvelopeExamplesValidate sanity-checks the vendored spec
// examples against the vendored envelope schema (fixture coherence).
func TestSpecEnvelopeExamplesValidate(t *testing.T) {
	sch := compileEnvelopeSchema(t)
	matches, err := filepath.Glob("testdata/examples/*.json")
	if err != nil || len(matches) == 0 {
		t.Fatalf("no envelope example fixtures found: %v", err)
	}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if err := sch.Validate(inst); err != nil {
			t.Errorf("%s fails envelope schema: %v", path, err)
		}
	}
}

// TestSpecEventExamplesValidate validates each vendored event example
// against its $defs payload schema.
func TestSpecEventExamplesValidate(t *testing.T) {
	defs := map[string]string{
		"user_received.json":     "UserReceivedPayload",
		"assistant_emitted.json": "AssistantEmittedPayload",
		"tool_called.json":       "ToolCalledPayload",
		"tool_completed.json":    "ToolCompletedPayload",
		"thinking_emitted.json":  "ThinkingEmittedPayload",
		"session_created.json":   "SessionCreatedPayload",
		"session_forked.json":    "SessionForkedPayload",
		"session_nested.json":    "SessionNestedPayload",
		"session_returned.json":  "SessionReturnedPayload",
	}
	for file, def := range defs {
		raw, err := os.ReadFile(filepath.Join("testdata/examples/events", file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("decode %s: %v", file, err)
		}
		if err := compilePayloadSchema(t, def).Validate(inst); err != nil {
			t.Errorf("%s fails %s: %v", file, def, err)
		}
	}
}

// TestEnvelopeTypeRoundTripsSpecExamples proves the local Envelope
// type surface carries spec-shaped envelopes without dropping fields:
// unmarshal example → re-marshal → still schema-valid, linkage fields
// and content bytes intact.
func TestEnvelopeTypeRoundTripsSpecExamples(t *testing.T) {
	sch := compileEnvelopeSchema(t)
	matches, _ := filepath.Glob("testdata/examples/*.json")
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		var env Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("%s: unmarshal into producer Envelope: %v", path, err)
		}
		out, err := json.Marshal(&env)
		if err != nil {
			t.Fatalf("%s: re-marshal: %v", path, err)
		}

		inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(out))
		if err != nil {
			t.Fatalf("%s: decode round-tripped: %v", path, err)
		}
		if err := sch.Validate(inst); err != nil {
			t.Errorf("%s: round-tripped envelope fails schema: %v\n%s", path, err, out)
		}

		var orig, rt map[string]any
		if err := json.Unmarshal(raw, &orig); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(out, &rt); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"id", "parent_id", "fork_point", "crtx_version"} {
			if want, ok := orig[key]; ok {
				if got := rt[key]; got != want {
					t.Errorf("%s: round-trip dropped/changed %s: %v -> %v", path, key, want, got)
				}
			}
		}
		origTurns := orig["turns"].([]any)
		if len(env.Turns) != len(origTurns) {
			t.Errorf("%s: turn count %d -> %d", path, len(origTurns), len(env.Turns))
		}
	}
}

// TestDispatchNestLinkageRoundTrip covers the child-envelope linkage
// fields (dispatched_from, injected_turns) with a locally built
// envelope: schema-valid on disk and readable through the consumer
// mirror with linkage intact.
func TestDispatchNestLinkageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(dir)

	if err := s.Create("child-1", time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
		Source{Kind: "stem", Version: "0.1.0"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.AppendTurn("child-1", testTurn("t1", "user", NewTextPart("dispatched work"))); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	// Producer-side linkage annotation (future dispatch-nest support
	// writes these fields; the type surface must already carry them).
	path := filepath.Join(dir, "child-1.jsonl")
	env, err := readEnvelope(path)
	if err != nil {
		t.Fatal(err)
	}
	env.DispatchedFrom = &DispatchedFrom{EnvelopeID: "parent-1", CallID: "call-9"}
	env.InjectedTurns = []InjectedTurnRef{{
		EnvelopeID: "parent-1", StartTurnID: "p-t1", EndTurnID: "p-t2",
	}}
	if err := writeEnvelope(path, env); err != nil {
		t.Fatal(err)
	}

	_, line := readSessionFile(t, path)
	validateEnvelopeBytes(t, line)

	got, err := consumerReadEnvelopeFile(path)
	if err != nil {
		t.Fatalf("consumer read: %v", err)
	}
	if got.DispatchedFrom == nil || got.DispatchedFrom.EnvelopeID != "parent-1" || got.DispatchedFrom.CallID != "call-9" {
		t.Errorf("dispatched_from not preserved: %+v", got.DispatchedFrom)
	}
	if len(got.InjectedTurns) != 1 || got.InjectedTurns[0].StartTurnID != "p-t1" {
		t.Errorf("injected_turns not preserved: %+v", got.InjectedTurns)
	}
}

// TestProducerOutputConsumableByConsumerReader is the end-to-end
// interop check: hook sequence → translator → bus → recorder →
// session JSONL → consumer-mirror scan/parse, with schema validation
// on the same bytes.
func TestProducerOutputConsumableByConsumerReader(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	b := kitbus.New()
	defer func() { _ = b.Close(context.Background()) }()

	rec := NewRecorder(store)
	defer rec.Attach(b)()

	publishAll(t, b,
		hookEnv("SessionStart", "ev-1", `{"session_id":"9f36c8a2-6c5f-4a51-b2f4-1f2f3a4b5c6d"}`),
		hookEnv("UserPromptSubmit", "ev-2", `{"prompt":"ship it"}`),
		hookEnv("PreToolUse", "ev-3", `{"tool_name":"Bash","tool_input":{"command":"make test"}}`),
		hookEnv("PostToolUse", "ev-4", `{"tool_name":"Bash","tool_input":{"command":"make test"},"tool_response":{"exit":0}}`),
		hookEnv("Stop", "ev-5", `{"stop_reason":"end_turn","turn_index":1}`),
		hookEnv("SessionEnd", "ev-6", `{"session_id":"9f36c8a2-6c5f-4a51-b2f4-1f2f3a4b5c6d","exit_code":0}`),
	)

	files, err := consumerScanDir(dir)
	if err != nil {
		t.Fatalf("consumer scan: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("consumer scan found %d envelope files, want 1: %v", len(files), files)
	}

	env, err := consumerReadEnvelopeFile(files[0])
	if err != nil {
		t.Fatalf("consumer parse (a failure here means the indexer would silently skip us): %v", err)
	}
	if env.CrtxVersion != consumerSupportedVersion {
		t.Fatalf("crtx_version = %q; consumer gates on %q and would skip the file",
			env.CrtxVersion, consumerSupportedVersion)
	}
	if env.ID != "9f36c8a2-6c5f-4a51-b2f4-1f2f3a4b5c6d" {
		t.Errorf("id = %q", env.ID)
	}
	if env.Source.Kind != "claude-code" || env.Source.Version == "" {
		t.Errorf("source = %+v; consumer surfaces kind/version for detection", env.Source)
	}
	if env.UpdatedAt.Before(env.CreatedAt) {
		t.Errorf("updated_at %v before created_at %v", env.UpdatedAt, env.CreatedAt)
	}

	wantRoles := []string{"user", "assistant", "tool", "assistant"}
	if len(env.Turns) != len(wantRoles) {
		t.Fatalf("turns = %d, want %d", len(env.Turns), len(wantRoles))
	}
	for i, want := range wantRoles {
		if env.Turns[i].Role != want {
			t.Errorf("turns[%d].role = %q, want %q", i, env.Turns[i].Role, want)
		}
		if len(env.Turns[i].Content) == 0 {
			t.Errorf("turns[%d] has no content parts", i)
		}
	}
	if kind := consumerPartKind(env.Turns[1].Content[0]); kind != "tool_call" {
		t.Errorf("turns[1] part kind = %q, want tool_call", kind)
	}
	if kind := consumerPartKind(env.Turns[2].Content[0]); kind != "tool_result" {
		t.Errorf("turns[2] part kind = %q, want tool_result", kind)
	}

	// Same bytes must also validate against the envelope schema — the
	// consumer's silent-skip tolerance must never be load-bearing.
	_, line := readSessionFile(t, files[0])
	validateEnvelopeBytes(t, line)
}
