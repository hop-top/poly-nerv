// Package testutil provides cassette-based test helpers for nerv.
//
// Cassettes are deterministic JSON fixtures stored under testdata/cassettes/.
// They capture input/expected pairs for replay-based testing.
//
// Two modes:
//   - Replay (default): assert actual output matches cassette content.
//   - Record (NERV_RECORD=1): write actual output as new cassette content.
package testutil

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// cassetteRoot returns the absolute path to testdata/cassettes.
func cassetteRoot() string {
	_, f, _, _ := runtime.Caller(0)
	// internal/testutil/cassette.go -> ../../testdata/cassettes
	return filepath.Join(filepath.Dir(f), "..", "..", "testdata", "cassettes")
}

// CassetteDir returns the absolute path to testdata/cassettes/<subdir>.
func CassetteDir(subdir string) string {
	return filepath.Join(cassetteRoot(), subdir)
}

// LoadCassette reads a cassette JSON file from testdata/cassettes/<path>.
// It calls t.Fatal on read errors.
func LoadCassette(t testing.TB, path string) []byte {
	t.Helper()
	full := filepath.Join(cassetteRoot(), path)
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("LoadCassette(%q): %v", path, err)
	}
	return data
}

// GoldenCassette compares actual output to cassette content.
//
// In record mode (NERV_RECORD=1), it writes actual as the new cassette.
// In replay mode (default), it asserts byte-level equality after
// normalising JSON whitespace.
func GoldenCassette(t testing.TB, path string, actual []byte) {
	t.Helper()
	full := filepath.Join(cassetteRoot(), path)

	if os.Getenv("NERV_RECORD") == "1" {
		// Normalise to indented JSON before writing.
		var buf bytes.Buffer
		if err := json.Indent(&buf, actual, "", "  "); err == nil {
			actual = append(buf.Bytes(), '\n')
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("GoldenCassette mkdir: %v", err)
		}
		if err := os.WriteFile(full, actual, 0o644); err != nil {
			t.Fatalf("GoldenCassette write: %v", err)
		}
		return
	}

	want, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("GoldenCassette read(%q): %v\nRun with NERV_RECORD=1 to create.", path, err)
	}

	// Normalise both sides for comparison.
	normWant := normaliseJSON(want)
	normGot := normaliseJSON(actual)

	if !bytes.Equal(normWant, normGot) {
		t.Errorf("GoldenCassette(%q) mismatch\n--- want ---\n%s\n--- got ---\n%s",
			path, normWant, normGot)
	}
}

// normaliseJSON compacts then re-indents JSON for stable comparison.
// Returns input unchanged if it is not valid JSON.
func normaliseJSON(data []byte) []byte {
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return data
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, compact.Bytes(), "", "  "); err != nil {
		return compact.Bytes()
	}
	return pretty.Bytes()
}
