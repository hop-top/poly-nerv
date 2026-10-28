package guardrails

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectKey_StandardRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "src", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(sub); got != root {
		t.Errorf("ProjectRoot(%q) = %q, want %q", sub, got, root)
	}
	if k := ProjectKey(sub); len(k) != 16 {
		t.Errorf("ProjectKey returned %q (len=%d), want 16-char hex", k, len(k))
	}
}

func TestProjectKey_NoRepo(t *testing.T) {
	dir := t.TempDir()
	if got := ProjectRoot(dir); got != dir {
		t.Errorf("ProjectRoot(%q) = %q, want %q (no repo fallback)", dir, got, dir)
	}
}

// TestProjectKey_Worktree models the git-hop layout: bare repo at
// /tmp/<repo>/, hops at /tmp/<repo>/hops/<branch>/, .git in each hop
// is a gitfile pointing back at /tmp/<repo>/worktrees/<branch>.
//
// Worktrees of the same bare repo MUST collapse to one ProjectKey.
func TestProjectKey_Worktree(t *testing.T) {
	bare := t.TempDir()
	worktreesDir := filepath.Join(bare, "worktrees")

	makeWorktree := func(name string) string {
		wt := filepath.Join(bare, "hops", name)
		if err := os.MkdirAll(wt, 0o755); err != nil {
			t.Fatal(err)
		}
		gitdir := filepath.Join(worktreesDir, name)
		if err := os.MkdirAll(gitdir, 0o755); err != nil {
			t.Fatal(err)
		}
		gitfile := filepath.Join(wt, ".git")
		body := []byte("gitdir: " + gitdir + "\n")
		if err := os.WriteFile(gitfile, body, 0o644); err != nil {
			t.Fatal(err)
		}
		return wt
	}

	wtA := makeWorktree("feat-a")
	wtB := makeWorktree("feat-b")

	rootA := ProjectRoot(wtA)
	rootB := ProjectRoot(wtB)
	if rootA != bare {
		t.Errorf("worktree A ProjectRoot = %q, want %q", rootA, bare)
	}
	if rootB != bare {
		t.Errorf("worktree B ProjectRoot = %q, want %q", rootB, bare)
	}
	if ProjectKey(wtA) != ProjectKey(wtB) {
		t.Error("two worktrees of the same bare repo got different ProjectKeys")
	}
}
