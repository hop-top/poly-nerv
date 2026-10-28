package hookpkg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallAndRemove(t *testing.T) {
	tmp := t.TempDir()

	// Create a fake handler source file.
	srcHandler := filepath.Join(tmp, "handler.sh")
	if err := os.WriteFile(srcHandler, []byte("#!/bin/sh\necho ok"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Override XDG_DATA_HOME so install writes to tmp.
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))

	// Work inside a project dir.
	projDir := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(projDir)

	// Install.
	if err := Install("test-hook", srcHandler); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Verify handler exists.
	handlerPath := filepath.Join(tmp, "data", "nerv", "hooks", "test-hook", "handler")
	if _, err := os.Stat(handlerPath); err != nil {
		t.Fatalf("handler not found: %v", err)
	}

	// Verify lockfile.
	lf, err := ReadLockfile(filepath.Join(projDir, ".nerv", "hooks.lock"))
	if err != nil {
		t.Fatalf("ReadLockfile: %v", err)
	}
	if !lf.Has("test-hook") {
		t.Fatal("lockfile missing test-hook")
	}
	if lf.Hooks[0].SHA256 == "" {
		t.Fatal("lockfile sha256 is empty")
	}

	// Verify hooks.yaml.
	yamlPath := filepath.Join(projDir, ".nerv", "hooks.yaml")
	if _, err := os.Stat(yamlPath); err != nil {
		t.Fatalf("hooks.yaml not found: %v", err)
	}

	// Remove (no purge).
	if err := Remove("test-hook", false); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	lf, _ = ReadLockfile(filepath.Join(projDir, ".nerv", "hooks.lock"))
	if lf.Has("test-hook") {
		t.Fatal("lockfile still has test-hook after remove")
	}

	// Handler files should still exist (no purge).
	if _, err := os.Stat(handlerPath); err != nil {
		t.Fatal("handler files removed without purge")
	}

	// Re-install and remove with purge.
	if err := Install("test-hook", srcHandler); err != nil {
		t.Fatalf("re-Install: %v", err)
	}
	if err := Remove("test-hook", true); err != nil {
		t.Fatalf("Remove purge: %v", err)
	}
	if _, err := os.Stat(handlerPath); !os.IsNotExist(err) {
		t.Fatal("handler files not purged")
	}
}

func TestLockfileAddRemove(t *testing.T) {
	lf := &Lockfile{}

	lf.Add(LockEntry{Name: "a", Version: "1.0.0"})
	lf.Add(LockEntry{Name: "b", Version: "2.0.0"})

	if len(lf.Hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(lf.Hooks))
	}

	// Update existing.
	lf.Add(LockEntry{Name: "a", Version: "1.1.0"})
	if len(lf.Hooks) != 2 {
		t.Fatalf("expected 2 hooks after update, got %d", len(lf.Hooks))
	}
	if lf.Hooks[0].Version != "1.1.0" {
		t.Fatalf("expected version 1.1.0, got %s", lf.Hooks[0].Version)
	}

	if !lf.Remove("a") {
		t.Fatal("Remove returned false for existing entry")
	}
	if lf.Remove("a") {
		t.Fatal("Remove returned true for missing entry")
	}
	if lf.Has("a") {
		t.Fatal("Has returned true after remove")
	}
}

func TestLockfileReadWrite(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "hooks.lock")

	// Read non-existent → empty.
	lf, err := ReadLockfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(lf.Hooks) != 0 {
		t.Fatal("expected empty lockfile")
	}

	lf.Add(LockEntry{Name: "x", Version: "1.0.0", SHA256: "abc"})
	if err := WriteLockfile(path, lf); err != nil {
		t.Fatal(err)
	}

	lf2, err := ReadLockfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !lf2.Has("x") {
		t.Fatal("round-trip failed")
	}
	if lf2.Hooks[0].SHA256 != "abc" {
		t.Fatalf("sha256 mismatch: %s", lf2.Hooks[0].SHA256)
	}
}
