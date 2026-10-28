package crtx

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const envelopeSchemaURL = "https://spec.hop.top/crtx/v0.1/envelope.schema.json"

func compileEnvelopeSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	raw, err := os.ReadFile("testdata/envelope.schema.json")
	if err != nil {
		t.Fatalf("read vendored envelope schema: %v", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode vendored envelope schema: %v", err)
	}

	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource(envelopeSchemaURL, doc); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	sch, err := c.Compile(envelopeSchemaURL)
	if err != nil {
		t.Fatalf("compile envelope schema: %v", err)
	}
	return sch
}

// readSessionFile reads a session file, asserting the one-line-per-file
// convention, and returns both the decoded envelope and the raw line.
func readSessionFile(t *testing.T, path string) (*Envelope, []byte) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read session file: %v", err)
	}
	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("session file has %d lines, want exactly 1 (stem JSONLStore convention)", len(lines))
	}

	var env Envelope
	if err := json.Unmarshal(lines[0], &env); err != nil {
		t.Fatalf("unmarshal envelope line: %v", err)
	}
	return &env, lines[0]
}

func validateEnvelopeBytes(t *testing.T, line []byte) {
	t.Helper()

	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(line))
	if err != nil {
		t.Fatalf("decode envelope for validation: %v", err)
	}
	if err := compileEnvelopeSchema(t).Validate(inst); err != nil {
		t.Fatalf("envelope failed schema validation: %v\nline: %s", err, line)
	}
}

func testTurn(id, role string, content ...any) Turn {
	parts := make([]json.RawMessage, 0, len(content))
	for _, c := range content {
		b, err := json.Marshal(c)
		if err != nil {
			panic(err)
		}
		parts = append(parts, b)
	}
	return Turn{
		ID:        id,
		Role:      role,
		CreatedAt: time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
		Content:   parts,
	}
}

func TestSessionStore_CreateThenAppend(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(dir)

	created := time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC)
	if err := s.Create("sess-1", created, Source{Kind: "claude-code", Version: "2.1.0"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.AppendTurn("sess-1", testTurn("t1", "user", NewTextPart("hi"))); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}
	if err := s.AppendTurn("sess-1", testTurn("t2", "assistant",
		ToolCallPart{Type: "tool_call", CallID: "c1", Name: "Bash", Input: map[string]any{"command": "ls"}})); err != nil {
		t.Fatalf("AppendTurn 2: %v", err)
	}

	env, line := readSessionFile(t, filepath.Join(dir, "sess-1.jsonl"))
	validateEnvelopeBytes(t, line)

	if env.CrtxVersion != Version {
		t.Errorf("crtx_version = %q, want %q", env.CrtxVersion, Version)
	}
	if env.ID != "sess-1" {
		t.Errorf("id = %q", env.ID)
	}
	if !env.CreatedAt.Equal(created) {
		t.Errorf("created_at = %v, want %v", env.CreatedAt, created)
	}
	if env.UpdatedAt.Before(env.CreatedAt) {
		t.Errorf("updated_at %v before created_at %v", env.UpdatedAt, env.CreatedAt)
	}
	if len(env.Turns) != 2 {
		t.Fatalf("turns = %d, want 2", len(env.Turns))
	}
	if env.Turns[0].Role != "user" || env.Turns[1].Role != "assistant" {
		t.Errorf("turn roles = %q,%q", env.Turns[0].Role, env.Turns[1].Role)
	}
}

func TestSessionStore_CreateIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(dir)

	if err := s.Create("sess-1", time.Now().UTC(), Source{Kind: "claude-code", Version: "1.0.0"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.AppendTurn("sess-1", testTurn("t1", "user", NewTextPart("hi"))); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}
	// A resumed session re-emits envelope.created; existing turns must
	// survive.
	if err := s.Create("sess-1", time.Now().UTC(), Source{Kind: "claude-code", Version: "1.0.0"}); err != nil {
		t.Fatalf("re-Create: %v", err)
	}

	env, _ := readSessionFile(t, filepath.Join(dir, "sess-1.jsonl"))
	if len(env.Turns) != 1 {
		t.Fatalf("turns after re-create = %d, want 1 (create must not truncate)", len(env.Turns))
	}
}

func TestSessionStore_AppendAutoCreatesSkeleton(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(dir)

	if err := s.AppendTurn("sess-2", testTurn("t1", "user", NewTextPart("late join"))); err != nil {
		t.Fatalf("AppendTurn: %v", err)
	}

	env, line := readSessionFile(t, filepath.Join(dir, "sess-2.jsonl"))
	validateEnvelopeBytes(t, line)

	if env.Metadata[MetaSynthesized] != true {
		t.Errorf("metadata[%s] = %v, want true", MetaSynthesized, env.Metadata[MetaSynthesized])
	}
	if env.Source.Kind == "" || env.Source.Version == "" {
		t.Errorf("skeleton source incomplete: %+v (schema minLength 1)", env.Source)
	}
}

func TestSessionStore_SetMetadata(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(dir)

	if err := s.Create("sess-3", time.Now().UTC(), Source{Kind: "codex", Version: "1.0.0"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.SetMetadata("sess-3", MetaSessionEnd, map[string]any{"exit_code": 0}); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}

	env, line := readSessionFile(t, filepath.Join(dir, "sess-3.jsonl"))
	validateEnvelopeBytes(t, line)
	if env.Metadata[MetaSessionEnd] == nil {
		t.Errorf("metadata[%s] missing", MetaSessionEnd)
	}
}

func TestSessionStore_RejectsUnsafeIDs(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(dir)

	for _, id := range []string{"", "a/b", "../escape", "a\\b", "."} {
		if err := s.AppendTurn(id, testTurn("t", "user", NewTextPart("x"))); err == nil {
			t.Errorf("AppendTurn(%q): want error for unsafe session id", id)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("unsafe ids materialized files: %v", entries)
	}
}

func TestSessionStore_EmptyTurnsMarshalsAsArray(t *testing.T) {
	dir := t.TempDir()
	s := NewSessionStore(dir)

	if err := s.Create("sess-4", time.Now().UTC(), Source{Kind: "stem", Version: "0.1.0"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, line := readSessionFile(t, filepath.Join(dir, "sess-4.jsonl"))
	validateEnvelopeBytes(t, line) // schema requires turns: array, rejects null
	if !bytes.Contains(line, []byte(`"turns":[]`)) {
		t.Errorf("empty turns must serialize as [], got: %s", line)
	}
}
