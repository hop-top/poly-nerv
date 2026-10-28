package guardrails

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestEngine(t *testing.T) *BackupEngine {
	t.Helper()
	state := t.TempDir()
	return &BackupEngine{
		StateDir: state,
		Now:      func() time.Time { return time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC) },
	}
}

// readTarEntries returns the archive's entry names + sizes for
// inspection. Skips dirs.
func readTarEntries(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip: %v", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		if hdr.Typeflag == tar.TypeDir {
			continue
		}
		names = append(names, hdr.Name)
	}
	return names
}

func TestBackup_PathArchive(t *testing.T) {
	cwd := t.TempDir()
	target := filepath.Join(cwd, "data")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(target, f), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	eng := newTestEngine(t)
	res, err := eng.Capture(BackupRequest{
		RuleID:   "rm-rf-glob",
		Strategy: "path_archive",
		CWD:      cwd,
		Command:  "rm -rf data",
	})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if res.Path == "" || res.Bytes == 0 {
		t.Errorf("empty result: %+v", res)
	}

	names := readTarEntries(t, res.Path)
	hasA, hasB := false, false
	for _, n := range names {
		if strings.HasSuffix(n, "a.txt") {
			hasA = true
		}
		if strings.HasSuffix(n, "b.txt") {
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Errorf("archive missing files: %v", names)
	}
}

func TestBackup_PathArchive_NoTarget(t *testing.T) {
	eng := newTestEngine(t)
	_, err := eng.Capture(BackupRequest{
		RuleID:   "rm-rf-glob",
		Strategy: "path_archive",
		CWD:      t.TempDir(),
		Command:  "rm -rf",
	})
	if err == nil {
		t.Error("expected error for empty path target list")
	}
}

func TestBackup_ScriptCapture_NoURL(t *testing.T) {
	eng := newTestEngine(t)
	_, err := eng.Capture(BackupRequest{
		RuleID:   "curl-pipe-shell",
		Strategy: "script_capture",
		CWD:      t.TempDir(),
		Command:  "echo hi | sh",
	})
	if err == nil {
		t.Error("expected error when no URL present")
	}
}

func TestBackup_UnknownStrategy(t *testing.T) {
	eng := newTestEngine(t)
	_, err := eng.Capture(BackupRequest{
		RuleID:   "x",
		Strategy: "bogus",
		CWD:      t.TempDir(),
		Command:  "echo hi",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown backup strategy") {
		t.Errorf("expected 'unknown backup strategy' error, got %v", err)
	}
}

func TestBackup_ArchivePathNamespacedByProject(t *testing.T) {
	eng := newTestEngine(t)
	cwdA := t.TempDir()
	cwdB := t.TempDir()
	for _, c := range []string{cwdA, cwdB} {
		f := filepath.Join(c, "f.txt")
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	resA, err := eng.Capture(BackupRequest{
		RuleID:   "rm-rf-glob",
		Strategy: "path_archive",
		CWD:      cwdA,
		Command:  "rm -rf f.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	resB, err := eng.Capture(BackupRequest{
		RuleID:   "rm-rf-glob",
		Strategy: "path_archive",
		CWD:      cwdB,
		Command:  "rm -rf f.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	dirA := filepath.Dir(resA.Path)
	dirB := filepath.Dir(resB.Path)
	if dirA == dirB {
		t.Errorf("two different cwds produced same project dir %q", dirA)
	}
	if !strings.Contains(dirA, "backups") || !strings.Contains(dirB, "backups") {
		t.Errorf("archive paths missing 'backups' segment: %q %q", dirA, dirB)
	}
}

func TestBackup_StateDirRequired(t *testing.T) {
	eng := &BackupEngine{Now: time.Now}
	_, err := eng.Capture(BackupRequest{
		Strategy: "path_archive",
		CWD:      t.TempDir(),
		Command:  "rm -rf x",
	})
	if err == nil || !strings.Contains(err.Error(), "StateDir empty") {
		t.Errorf("expected StateDir-empty error, got %v", err)
	}
}
