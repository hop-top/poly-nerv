package envelope

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"pgregory.net/rapid"
)

func genPayload() *rapid.Generator[json.RawMessage] {
	return rapid.Custom(func(t *rapid.T) json.RawMessage {
		m := make(map[string]string)
		n := rapid.IntRange(0, 5).Draw(t, "payload-keys")
		for i := 0; i < n; i++ {
			k := rapid.StringN(1, 20, 20).Draw(t, "key")
			v := rapid.StringN(0, 50, 50).Draw(t, "val")
			m[k] = v
		}
		b, _ := json.Marshal(m)
		return json.RawMessage(b)
	})
}

func genEnvelope() *rapid.Generator[*Envelope] {
	return rapid.Custom(func(t *rapid.T) *Envelope {
		env := &Envelope{
			NervVersion: rapid.SampledFrom([]string{"1", "1.0", "2"}).Draw(t, "version"),
			Event:       rapid.StringN(1, 30, 30).Draw(t, "event"),
			EventID:     rapid.StringN(1, 30, 30).Draw(t, "event-id"),
			Timestamp:   time.Now().Truncate(time.Second),
			CLI: CLIInfo{
				Name:    rapid.StringN(1, 20, 20).Draw(t, "cli-name"),
				Version: rapid.StringN(1, 10, 10).Draw(t, "cli-version"),
			},
			Session: SessionInfo{
				ID: rapid.StringN(1, 20, 20).Draw(t, "session-id"),
			},
			Payload:     genPayload().Draw(t, "payload"),
			Synthesized: rapid.Bool().Draw(t, "synthesized"),
		}
		return env
	})
}

func TestRapid_EncodeDecodeRoundtrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		original := genEnvelope().Draw(t, "env")

		var buf bytes.Buffer
		if err := Encode(&buf, original); err != nil {
			t.Fatalf("encode: %v", err)
		}

		decoded, err := Decode(&buf)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}

		// Compare event field exactly.
		if decoded.Event != original.Event {
			t.Fatalf("event mismatch: got %q, want %q",
				decoded.Event, original.Event)
		}

		// Compare full struct via JSON re-serialization (handles
		// time.Time formatting and json.RawMessage equality).
		origJSON, _ := json.Marshal(original)
		decJSON, _ := json.Marshal(decoded)

		var origMap, decMap map[string]any
		if err := json.Unmarshal(origJSON, &origMap); err != nil {
			t.Fatalf("unmarshal original: %v", err)
		}
		if err := json.Unmarshal(decJSON, &decMap); err != nil {
			t.Fatalf("unmarshal decoded: %v", err)
		}

		if !reflect.DeepEqual(origMap, decMap) {
			t.Fatalf("roundtrip mismatch:\n  orig: %s\n  dec:  %s",
				origJSON, decJSON)
		}
	})
}

func TestRapid_EventFieldPreserved(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		event := rapid.StringN(1, 50, 50).Draw(t, "event")
		env := &Envelope{
			NervVersion: "1",
			Event:       event,
			EventID:     "test",
			Timestamp:   time.Now().Truncate(time.Second),
			CLI:         CLIInfo{Name: "test", Version: "1"},
			Session:     SessionInfo{ID: "s1"},
			Payload:     json.RawMessage(`{}`),
		}

		var buf bytes.Buffer
		if err := Encode(&buf, env); err != nil {
			t.Fatalf("encode: %v", err)
		}

		decoded, err := Decode(&buf)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}

		if decoded.Event != event {
			t.Fatalf("event not preserved: got %q, want %q",
				decoded.Event, event)
		}
	})
}
