package envelope

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

func testEnvelope() *Envelope {
	return &Envelope{
		NervVersion: "1.0.0-draft",
		Event:       "SessionStart",
		EventID:     "550e8400-e29b-41d4-a716-446655440000",
		Timestamp:   time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		CLI:         CLIInfo{Name: "claude-code", Version: "1.0.0"},
		Session:     SessionInfo{ID: "sess-001", CWD: "/tmp"},
		Payload:     json.RawMessage(`{"key":"value"}`),
	}
}

func TestEncode_Decode_Roundtrip(t *testing.T) {
	env := testEnvelope()

	var buf bytes.Buffer
	if err := Encode(&buf, env); err != nil {
		t.Fatalf("encode: %v", err)
	}

	got, err := Decode(&buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Event != env.Event {
		t.Errorf("event = %q, want %q", got.Event, env.Event)
	}
	if got.CLI.Name != env.CLI.Name {
		t.Errorf("cli.name = %q, want %q", got.CLI.Name, env.CLI.Name)
	}
	if got.Session.ID != env.Session.ID {
		t.Errorf("session.id = %q, want %q", got.Session.ID, env.Session.ID)
	}
	if string(got.Payload) != string(env.Payload) {
		t.Errorf("payload = %s, want %s", got.Payload, env.Payload)
	}
}

func TestDecode_TooLarge(t *testing.T) {
	// Build a payload just over 1 MB.
	big := strings.Repeat("x", int(DefaultMaxEnvelopeSize)+100)
	raw := `{"nerv_version":"1.0.0","event":"SessionStart",` +
		`"event_id":"a","timestamp":"2025-01-01T00:00:00Z",` +
		`"cli":{"name":"c","version":"1"},"session":{"id":"s"},` +
		`"payload":{"d":"` + big + `"}}`

	_, err := Decode(strings.NewReader(raw))
	if err == nil {
		t.Fatal("expected error for oversized envelope")
	}
}

func TestDecode_EnvOverride(t *testing.T) {
	// Set a 2 MB limit; a 1.5 MB payload should pass.
	t.Setenv("NERV_MAX_ENVELOPE_SIZE", "2097152")

	filler := strings.Repeat("a", 1_400_000)
	raw := `{"nerv_version":"1.0.0","event":"SessionStart",` +
		`"event_id":"a","timestamp":"2025-01-01T00:00:00Z",` +
		`"cli":{"name":"c","version":"1"},"session":{"id":"s"},` +
		`"payload":{"d":"` + filler + `"}}`

	env, err := Decode(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("decode should succeed with 2MB limit: %v", err)
	}
	if env.Event != "SessionStart" {
		t.Errorf("event = %q, want SessionStart", env.Event)
	}
}

func TestDecode_HardCeiling(t *testing.T) {
	// Even with env set to 20 MB, hard ceiling is 10 MB.
	t.Setenv("NERV_MAX_ENVELOPE_SIZE", "20971520")

	if got := resolveMaxSize(); got != HardMaxEnvelopeSize {
		t.Errorf("resolveMaxSize = %d, want %d", got, HardMaxEnvelopeSize)
	}

	// 11 MB payload must be rejected.
	big := strings.Repeat("z", 11<<20)
	raw := `{"nerv_version":"1.0.0","event":"SessionStart",` +
		`"event_id":"a","timestamp":"2025-01-01T00:00:00Z",` +
		`"cli":{"name":"c","version":"1"},"session":{"id":"s"},` +
		`"payload":{"d":"` + big + `"}}`

	_, err := Decode(strings.NewReader(raw))
	if err == nil {
		t.Fatal("expected error for >10MB envelope")
	}
}

// The decoder may stop reading right after the JSON value, leaving the
// encoder's trailing newline unread. A leftover byte within the limit is
// not overflow: Decode must accept the encoded form however r chunks it.
func TestDecode_TrailingNewlineUnread(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, testEnvelope()); err != nil {
		t.Fatalf("encode: %v", err)
	}

	env, err := Decode(iotest.OneByteReader(&buf))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Event != "SessionStart" {
		t.Errorf("event = %q, want SessionStart", env.Event)
	}
}

// The limit covers every byte of input, trailing whitespace included.
func TestDecode_LimitCountsTrailingBytes(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, testEnvelope()); err != nil {
		t.Fatalf("encode: %v", err)
	}
	raw := buf.Bytes()

	t.Setenv("NERV_MAX_ENVELOPE_SIZE", strconv.Itoa(len(raw)))
	if _, err := Decode(iotest.OneByteReader(bytes.NewReader(raw))); err != nil {
		t.Fatalf("input of exactly the limit must decode: %v", err)
	}

	t.Setenv("NERV_MAX_ENVELOPE_SIZE", strconv.Itoa(len(raw)-1))
	if _, err := Decode(iotest.OneByteReader(bytes.NewReader(raw))); err == nil {
		t.Fatal("input one byte over the limit must fail")
	}
}

func TestDecode_InvalidJSON(t *testing.T) {
	_, err := Decode(strings.NewReader(`{not valid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestEncode_StreamingOutput(t *testing.T) {
	// Verify Encode writes directly to the writer (streaming)
	// by using a writer that counts Write calls.
	env := testEnvelope()
	cw := &countWriter{}
	if err := Encode(cw, env); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if cw.calls == 0 {
		t.Error("expected at least one Write call")
	}
	// Verify valid JSON was produced.
	var check Envelope
	if err := json.Unmarshal(cw.buf.Bytes(), &check); err != nil {
		t.Errorf("output is not valid JSON: %v", err)
	}
}

// countWriter wraps a buffer and counts Write calls.
type countWriter struct {
	buf   bytes.Buffer
	calls int
}

func (w *countWriter) Write(p []byte) (int, error) {
	w.calls++
	return w.buf.Write(p)
}

// Ensure countWriter satisfies io.Writer.
var _ io.Writer = (*countWriter)(nil)
