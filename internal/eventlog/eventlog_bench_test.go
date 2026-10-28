package eventlog

import (
	"encoding/json"
	"testing"
	"time"

	"hop.top/nerv/internal/envelope"
)

// BenchmarkJSONLWrite measures single event append to a JSONL log.
func BenchmarkJSONLWrite(b *testing.B) {
	dir := b.TempDir()
	logger := &JSONLLogger{Dir: dir}

	env := &envelope.Envelope{
		NervVersion: "1",
		Event:       "pre-tool-use",
		EventID:     "bench-evt-001",
		Timestamp:   time.Now(),
		CLI:         envelope.CLIInfo{Name: "bench", Version: "0.1.0"},
		Session:     envelope.SessionInfo{ID: "sess-bench"},
		Payload:     json.RawMessage(`{"tool":"bash","input":"echo hi"}`),
	}
	dec := map[string]string{"action": "allow"}

	b.ResetTimer()
	for range b.N {
		if err := logger.Write(env, dec); err != nil {
			b.Fatal(err)
		}
	}
}
