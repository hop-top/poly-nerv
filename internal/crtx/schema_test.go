package crtx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestValidateEnvelopeBytes_SpecExamples proves the runtime validator
// enforces the same vendored contract the test-only compilers pin:
// every vendored spec example passes.
func TestValidateEnvelopeBytes_SpecExamples(t *testing.T) {
	matches, err := filepath.Glob("testdata/examples/*.json")
	if err != nil || len(matches) == 0 {
		t.Fatalf("no envelope example fixtures found: %v", err)
	}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if err := ValidateEnvelopeBytes(raw); err != nil {
			t.Errorf("%s rejected by runtime validator: %v", path, err)
		}
	}
}

// TestValidateEnvelopeBytes_Rejects covers the drop-with-account
// boundary inputs: malformed JSON, wrong version-era shapes, and
// role values outside the v0.1 enum (envelope spec: reject, never
// coerce).
func TestValidateEnvelopeBytes_Rejects(t *testing.T) {
	cases := map[string]string{
		"not json":     `{"crtx_version": "0.1",`,
		"empty object": `{}`,
		"missing turns": `{"crtx_version":"0.1","id":"e1",
			"created_at":"2026-08-08T10:00:00Z","updated_at":"2026-08-08T10:00:00Z",
			"source":{"kind":"stem","version":"0.1.0"}}`,
		"unknown role": `{"crtx_version":"0.1","id":"e1",
			"created_at":"2026-08-08T10:00:00Z","updated_at":"2026-08-08T10:00:00Z",
			"source":{"kind":"stem","version":"0.1.0"},
			"turns":[{"id":"t1","role":"robot","created_at":"2026-08-08T10:00:00Z",
			"content":[{"type":"text","text":"hi"}]}]}`,
	}
	for name, doc := range cases {
		if err := ValidateEnvelopeBytes([]byte(doc)); err == nil {
			t.Errorf("%s: validator accepted invalid envelope", name)
		}
	}
}

// TestValidateEventPayload_SpecExamples validates each vendored event
// example through the topic-keyed runtime entry point.
func TestValidateEventPayload_SpecExamples(t *testing.T) {
	files := map[string]string{
		"user_received.json":     TopicUserReceived,
		"assistant_emitted.json": TopicAssistantEmitted,
		"tool_called.json":       TopicToolCalled,
		"tool_completed.json":    TopicToolCompleted,
		"thinking_emitted.json":  TopicThinkingEmitted,
		"session_created.json":   TopicSessionCreated,
		"session_forked.json":    TopicSessionForked,
		"session_nested.json":    TopicSessionNested,
		"session_returned.json":  TopicSessionReturned,
	}
	for file, topic := range files {
		raw, err := os.ReadFile(filepath.Join("testdata/examples/events", file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		var payload any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decode %s: %v", file, err)
		}
		if err := ValidateEventPayload(topic, payload); err != nil {
			t.Errorf("%s rejected for %s: %v", file, topic, err)
		}
	}
}

// TestValidateEventPayload_Rejects exercises the failure arms: missing
// required identity fields, wrong content shape, and topics without a
// canonical payload schema.
func TestValidateEventPayload_Rejects(t *testing.T) {
	if err := ValidateEventPayload(TopicUserReceived, map[string]any{
		"turn_id": "t1",
		"content": []any{map[string]any{"type": "text", "text": "hi"}},
	}); err == nil {
		t.Error("payload missing envelope_id accepted")
	}

	if err := ValidateEventPayload(TopicUserReceived, map[string]any{
		"envelope_id": "e1",
		"turn_id":     "t1",
		"content":     "not an array",
	}); err == nil {
		t.Error("payload with non-array content accepted")
	}

	if err := ValidateEventPayload("x.nerv.session.ended", map[string]any{}); err == nil {
		t.Error("extension topic accepted by canonical validator")
	}
	if err := ValidateEventPayload("crtx.turn.user.imagined", map[string]any{}); err == nil {
		t.Error("unknown crtx topic accepted by canonical validator")
	}
}
