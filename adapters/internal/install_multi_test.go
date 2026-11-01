package internal

import (
	"os"
	"path/filepath"
	"testing"
)

type multiMock struct {
	mockAdapter
	files map[string][]byte
	err   error
}

func (m *multiMock) MergeFiles(read FileReader) (map[string][]byte, error) {
	return m.files, m.err
}

func newMulti(dir string) *multiMock {
	cfg := filepath.Join(dir, "cfg.json")
	plug := filepath.Join(dir, "sub", "plugin.ts")
	stale := filepath.Join(dir, "old.ts")
	return &multiMock{
		mockAdapter: mockAdapter{
			cli:          "multi-cli",
			detectResult: DetectResult{Found: true, Info: CLIInfo{Version: "1.0.0"}},
			configPaths:  []string{cfg, plug},
		},
		files: map[string][]byte{
			cfg: []byte("cfg"), plug: []byte("plugin"), stale: nil,
		},
	}
}

func TestInstallMulti_WritesAllFilesAndRemovesStale(t *testing.T) {
	dir := t.TempDir()
	inst := newInstallerAt(filepath.Join(dir, "state"))
	a := newMulti(dir)
	stale := filepath.Join(dir, "old.ts")
	if err := os.WriteFile(stale, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := inst.Install(a, false); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		filepath.Join(dir, "cfg.json"):         "cfg",
		filepath.Join(dir, "sub", "plugin.ts"): "plugin",
	} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Errorf("%s = %q, %v; want %q", path, got, err, want)
		}
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale file not removed")
	}
	recs, _ := inst.Journal("multi-cli")
	if len(recs) != 1 || len(recs[0].Files) != 3 {
		t.Fatalf("journal = %+v", recs)
	}
	if drift, err := inst.DetectDrift(a); err != nil || drift {
		t.Errorf("drift = %v, %v; want none", drift, err)
	}
}

func TestInstallMulti_RerunWritesNoJournal(t *testing.T) {
	dir := t.TempDir()
	inst := newInstallerAt(filepath.Join(dir, "state"))
	a := newMulti(dir)
	for i := 0; i < 2; i++ {
		if err := inst.Install(a, false); err != nil {
			t.Fatal(err)
		}
	}
	if recs, _ := inst.Journal("multi-cli"); len(recs) != 1 {
		t.Errorf("journal len = %d, want 1 (no-op re-run)", len(recs))
	}
}

func TestInstallMulti_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	inst := newInstallerAt(filepath.Join(dir, "state"))
	if err := inst.Install(newMulti(dir), true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "cfg.json")); !os.IsNotExist(err) {
		t.Error("dry run wrote a file")
	}
}

func TestInstallMulti_RollsBackOnWriteFailure(t *testing.T) {
	dir := t.TempDir()
	inst := newInstallerAt(filepath.Join(dir, "state"))
	a := newMulti(dir)
	cfg := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfg, []byte("orig"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A file where the plugin's parent directory must go: write fails.
	if err := os.WriteFile(filepath.Join(dir, "sub"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := inst.Install(a, false); err == nil {
		t.Fatal("install succeeded despite unwritable plugin path")
	}
	if got, _ := os.ReadFile(cfg); string(got) != "orig" {
		t.Errorf("cfg = %q after failed install, want rollback to orig", got)
	}
	if recs, _ := inst.Journal("multi-cli"); len(recs) != 0 {
		t.Errorf("journal written for failed install: %+v", recs)
	}
}

func TestInstallMulti_UninstallRestoresPreState(t *testing.T) {
	dir := t.TempDir()
	inst := newInstallerAt(filepath.Join(dir, "state"))
	a := newMulti(dir)
	cfg := filepath.Join(dir, "cfg.json")
	stale := filepath.Join(dir, "old.ts")
	_ = os.WriteFile(cfg, []byte("orig"), 0o600)
	_ = os.WriteFile(stale, []byte("old"), 0o600)

	if err := inst.Install(a, false); err != nil {
		t.Fatal(err)
	}
	if err := inst.Uninstall(a); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(cfg); string(got) != "orig" {
		t.Errorf("cfg = %q, want orig", got)
	}
	if got, _ := os.ReadFile(stale); string(got) != "old" {
		t.Errorf("stale = %q, want old restored", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "plugin.ts")); !os.IsNotExist(err) {
		t.Error("created plugin not removed on uninstall")
	}
}

func TestInstallMulti_DriftWhenFileEdited(t *testing.T) {
	dir := t.TempDir()
	inst := newInstallerAt(filepath.Join(dir, "state"))
	a := newMulti(dir)
	if err := inst.Install(a, false); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "sub", "plugin.ts"), []byte("edited"), 0o600)
	if drift, _ := inst.DetectDrift(a); !drift {
		t.Error("edited plugin not reported as drift")
	}
}
