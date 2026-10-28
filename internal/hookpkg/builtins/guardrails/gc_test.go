package guardrails

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeArchive creates a fake archive with given mtime + size. The
// content is throwaway; GC only cares about file metadata.
func makeArchive(t *testing.T, dir string, name string, size int64, mtime time.Time) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGC_TTLDeletesOldArchives(t *testing.T) {
	root := t.TempDir()
	old := time.Now().Add(-72 * time.Hour) // older than 48h TTL
	fresh := time.Now().Add(-1 * time.Hour)
	pdir := filepath.Join(root, "backups", "proj1")
	oldArchive := makeArchive(t, pdir, "old.tar.gz", 100, old)
	freshArchive := makeArchive(t, pdir, "fresh.tar.gz", 100, fresh)

	stats, err := RunGC(root, GCConfig{TTL: 48 * time.Hour})
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	if stats.DeletedTTL != 1 {
		t.Errorf("DeletedTTL = %d, want 1", stats.DeletedTTL)
	}
	if _, err := os.Stat(oldArchive); !os.IsNotExist(err) {
		t.Errorf("old archive should be gone, got err=%v", err)
	}
	if _, err := os.Stat(freshArchive); err != nil {
		t.Errorf("fresh archive should remain, got err=%v", err)
	}
}

func TestGC_CountCapPerProject(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	pdir := filepath.Join(root, "backups", "proj1")
	for i := 0; i < 5; i++ {
		makeArchive(t, pdir,
			"a"+string(rune('0'+i))+".tar.gz",
			100,
			now.Add(time.Duration(-i)*time.Minute))
	}

	stats, err := RunGC(root, GCConfig{MaxPerProject: 2})
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	if stats.DeletedCount != 3 {
		t.Errorf("DeletedCount = %d, want 3", stats.DeletedCount)
	}
	left, err := os.ReadDir(pdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 {
		t.Errorf("survivors = %d, want 2", len(left))
	}
	// Verify the newest two survived (a0 and a1).
	for _, e := range left {
		if e.Name() != "a0.tar.gz" && e.Name() != "a1.tar.gz" {
			t.Errorf("unexpected survivor %q", e.Name())
		}
	}
}

func TestGC_SizeCapDeletesOldestFirst(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	pdir := filepath.Join(root, "backups", "proj1")
	// Three 1KB archives; size cap = 2KB → oldest gets pruned.
	makeArchive(t, pdir, "old.tar.gz", 1024, now.Add(-3*time.Minute))
	makeArchive(t, pdir, "mid.tar.gz", 1024, now.Add(-2*time.Minute))
	makeArchive(t, pdir, "new.tar.gz", 1024, now.Add(-1*time.Minute))

	stats, err := RunGC(root, GCConfig{MaxTotalBytes: 2048})
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	if stats.DeletedSize != 1 {
		t.Errorf("DeletedSize = %d, want 1", stats.DeletedSize)
	}
	if _, err := os.Stat(filepath.Join(pdir, "old.tar.gz")); !os.IsNotExist(err) {
		t.Errorf("old archive should be deleted (size cap), got err=%v", err)
	}
}

func TestGC_HybridAllPhases(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	// Two projects, each with one expired + two fresh.
	for _, proj := range []string{"a", "b"} {
		pdir := filepath.Join(root, "backups", proj)
		makeArchive(t, pdir, "expired.tar.gz", 1024, now.Add(-72*time.Hour))
		makeArchive(t, pdir, "older.tar.gz", 1024, now.Add(-2*time.Hour))
		makeArchive(t, pdir, "newer.tar.gz", 1024, now.Add(-1*time.Hour))
	}
	cfg := GCConfig{
		TTL:           48 * time.Hour,
		MaxPerProject: 1,
		MaxTotalBytes: 1024, // only 1 KB total → must drop one of the two
	}
	stats, err := RunGC(root, cfg)
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	// Phase 1: 2 expired removed.
	// Phase 2: 1 per project → drops 1 from each (2 total).
	// Phase 3: total now 2 KB > 1 KB → drops 1 more (oldest).
	if stats.DeletedTTL != 2 {
		t.Errorf("DeletedTTL = %d, want 2", stats.DeletedTTL)
	}
	if stats.DeletedCount != 2 {
		t.Errorf("DeletedCount = %d, want 2", stats.DeletedCount)
	}
	if stats.DeletedSize != 1 {
		t.Errorf("DeletedSize = %d, want 1", stats.DeletedSize)
	}
}

func TestGC_MissingRoot(t *testing.T) {
	stats, err := RunGC(t.TempDir(), GCConfig{TTL: 24 * time.Hour})
	if err != nil {
		t.Errorf("missing backups dir: got err=%v, want nil", err)
	}
	if stats.ArchivesScanned != 0 {
		t.Errorf("ArchivesScanned = %d, want 0", stats.ArchivesScanned)
	}
}

// Verify projectKey-namespaced layout matches what BackupEngine writes.
func TestGC_ConsistentLayoutWithBackup(t *testing.T) {
	root := t.TempDir()
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := &BackupEngine{StateDir: root, Now: time.Now}
	res, err := eng.Capture(BackupRequest{
		RuleID:   "rm-rf-glob",
		Strategy: "path_archive",
		CWD:      cwd,
		Command:  "rm -rf f.txt",
	})
	if err != nil {
		t.Fatal(err)
	}

	stats, err := RunGC(root, GCConfig{TTL: time.Nanosecond}) // anything older than now
	if err != nil {
		t.Fatal(err)
	}
	if stats.ArchivesScanned != 1 {
		t.Errorf("ArchivesScanned = %d, want 1", stats.ArchivesScanned)
	}
	// The newly-captured archive's mtime is "now", TTL is 1ns, so the
	// cutoff is essentially "anything older than now". The race
	// between Capture's mtime and RunGC's now means it may or may not
	// be deleted — assert it's NOT panicking + path is well-formed.
	if !strings.Contains(res.Path, filepath.Join("backups", ProjectKey(cwd))) {
		t.Errorf("backup path %q not under expected project dir", res.Path)
	}
}
