package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCassetteDir(t *testing.T) {
	dir := CassetteDir("dispatch")
	if !strings.HasSuffix(dir, filepath.Join("testdata", "cassettes", "dispatch")) {
		t.Errorf("CassetteDir(dispatch) = %q; want suffix testdata/cassettes/dispatch", dir)
	}
	// Directory must exist.
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("CassetteDir(dispatch) does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("CassetteDir(dispatch) is not a directory")
	}
}

func TestLoadCassette(t *testing.T) {
	data := LoadCassette(t, "dispatch/allow_no_hooks.json")
	if len(data) == 0 {
		t.Fatal("LoadCassette returned empty data")
	}
	if !strings.Contains(string(data), `"allow_no_hooks"`) {
		t.Error("LoadCassette content missing expected name field")
	}
}

func TestGoldenCassette_Replay(t *testing.T) {
	// Read the seed cassette, then assert it matches itself.
	data := LoadCassette(t, "dispatch/allow_no_hooks.json")
	GoldenCassette(t, "dispatch/allow_no_hooks.json", data)
}

func TestGoldenCassette_Mismatch(t *testing.T) {
	// Feed wrong content; expect test failure.
	fake := &fakeTB{}
	GoldenCassette(fake, "dispatch/allow_no_hooks.json", []byte(`{"wrong": true}`))
	if !fake.failed {
		t.Error("GoldenCassette should report mismatch for wrong content")
	}
}

func TestGoldenCassette_Record(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "record_test.json")

	// Temporarily override cassette root by writing to an absolute path
	// via the record-mode env var.
	t.Setenv("NERV_RECORD", "1")

	content := []byte(`{"recorded": true}`)
	// Use a sub-path that will be created under cassetteRoot.
	subPath := "scenarios/record_roundtrip.json"
	GoldenCassette(t, subPath, content)

	// Verify file was written.
	full := filepath.Join(cassetteRoot(), subPath)
	written, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("recorded file not found: %v", err)
	}
	if !strings.Contains(string(written), `"recorded"`) {
		t.Error("recorded file missing expected content")
	}

	// Clean up recorded file.
	_ = os.Remove(full)
	_ = os.Remove(path)
}

// fakeTB is a minimal testing.TB stand-in that captures failure state.
type fakeTB struct {
	testing.TB
	failed bool
}

func (f *fakeTB) Helper()                        {}
func (f *fakeTB) Fatalf(format string, a ...any) { f.failed = true }
func (f *fakeTB) Errorf(format string, a ...any) { f.failed = true }
