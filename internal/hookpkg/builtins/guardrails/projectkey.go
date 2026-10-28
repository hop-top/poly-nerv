package guardrails

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// ProjectKey returns a stable identifier for the repo containing cwd,
// used to namespace backup archives so worktrees of the same bare
// repo collapse to one bucket.
//
// Strategy (option a from kickoff):
//
//	walk up from cwd → first dir containing .git
//	  if .git is a directory → that dir is the project key
//	  if .git is a gitfile (worktrees) → resolve to bare root
//	  if no .git found → cwd itself
//
// The on-disk-friendly form is sha256(absPath)[:16] — 16 hex chars,
// no colons or slashes. Avoids leaking absolute paths into archive
// filenames while remaining deterministic across runs.
func ProjectKey(cwd string) string {
	root := projectRoot(cwd)
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:8]) // 16 hex chars
}

// ProjectRoot is the absolute path version (exposed for tests +
// diagnostics). Most callers should use ProjectKey for the
// filesystem-safe form.
func ProjectRoot(cwd string) string {
	return projectRoot(cwd)
}

func projectRoot(cwd string) string {
	dir := cwd
	for {
		gitPath := filepath.Join(dir, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				// Standard repo: .git is a dir; this is the root.
				return dir
			}
			// Worktree: .git is a gitfile pointing to the bare root.
			if root := readGitfile(gitPath); root != "" {
				return root
			}
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Hit filesystem root without finding .git; fall back to cwd.
			return cwd
		}
		dir = parent
	}
}

// readGitfile parses a `.git` file (used by worktrees) and returns
// the bare-root absolute path. Format: "gitdir: <path>\n". The
// referenced path typically points at <bare-root>/worktrees/<name>;
// we strip /worktrees/<name> to get the bare root proper.
func readGitfile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(s, prefix) {
		return ""
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(s, prefix))

	// Make absolute against the worktree's parent dir if relative.
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(filepath.Dir(path), gitdir)
	}

	// gitdir typically = <bare-root>/worktrees/<name>; strip both.
	// If it doesn't match, fall back to gitdir itself.
	parent := filepath.Dir(gitdir)
	if filepath.Base(parent) == "worktrees" {
		return filepath.Dir(parent)
	}
	return gitdir
}
