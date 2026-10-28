package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// CassetteEntry is a single recorded input/output pair.
type CassetteEntry struct {
	Input     *envelope.Envelope `json:"input"`
	Output    merger.Decision    `json:"output"`
	Timestamp time.Time          `json:"timestamp"`
}

// Recorder captures envelope+decision pairs as xrr cassettes.
// Thread-safe; entries appended via Record, flushed via Flush.
type Recorder struct {
	mu      sync.Mutex
	entries []CassetteEntry
}

// NewRecorder creates an empty Recorder.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// Record appends an envelope+decision pair to the cassette buffer.
func (r *Recorder) Record(
	env *envelope.Envelope, decision merger.Decision,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, CassetteEntry{
		Input:     env,
		Output:    decision,
		Timestamp: time.Now().UTC(),
	})
}

// Entries returns a copy of all recorded entries.
func (r *Recorder) Entries() []CassetteEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]CassetteEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

// Flush writes all recorded pairs to testdata/cassettes/<name>.json.
// Creates parent directories as needed. Returns the written path.
func (r *Recorder) Flush(name string) (string, error) {
	r.mu.Lock()
	entries := make([]CassetteEntry, len(r.entries))
	copy(entries, r.entries)
	r.mu.Unlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')

	dir := CassetteDir("")
	outPath := filepath.Join(dir, name+".json")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return "", err
	}
	return outPath, nil
}

// FlushTo writes all recorded pairs to the given absolute path.
func (r *Recorder) FlushTo(path string) error {
	r.mu.Lock()
	entries := make([]CassetteEntry, len(r.entries))
	copy(entries, r.entries)
	r.mu.Unlock()

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Reset clears all recorded entries.
func (r *Recorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = nil
}
