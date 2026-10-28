package envelope

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func makeEnvelope(payloadSize int) *Envelope {
	payload := strings.Repeat("x", payloadSize)
	raw, _ := json.Marshal(map[string]string{"data": payload})
	return &Envelope{
		NervVersion: "1",
		Event:       "pre-tool-use",
		EventID:     "bench-001",
		Timestamp:   time.Now(),
		CLI:         CLIInfo{Name: "bench-cli", Version: "0.1.0"},
		Session:     SessionInfo{ID: "sess-bench", CWD: "/tmp"},
		Payload:     json.RawMessage(raw),
	}
}

func benchEncode(b *testing.B, size int) {
	env := makeEnvelope(size)
	var buf bytes.Buffer
	b.ResetTimer()
	for range b.N {
		buf.Reset()
		if err := Encode(&buf, env); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode_Small(b *testing.B)  { benchEncode(b, 100) }
func BenchmarkEncode_Medium(b *testing.B) { benchEncode(b, 10_000) }
func BenchmarkEncode_Large(b *testing.B)  { benchEncode(b, 500_000) }

func benchDecode(b *testing.B, size int) {
	env := makeEnvelope(size)
	var buf bytes.Buffer
	if err := Encode(&buf, env); err != nil {
		b.Fatal(err)
	}
	data := buf.Bytes()

	b.ResetTimer()
	for range b.N {
		r := bytes.NewReader(data)
		if _, err := Decode(r); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecode_Small(b *testing.B)  { benchDecode(b, 100) }
func BenchmarkDecode_Medium(b *testing.B) { benchDecode(b, 10_000) }
func BenchmarkDecode_Large(b *testing.B)  { benchDecode(b, 500_000) }
