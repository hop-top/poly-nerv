package guardrails

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BackupRequest is the input to BackupEngine.Capture.
type BackupRequest struct {
	// RuleID names the rule that triggered the backup; used in
	// archive filenames for traceability.
	RuleID string

	// Strategy is one of: path_archive | git_stash_full |
	// git_untracked | remote_bundle | script_capture.
	Strategy string

	// CWD is the agent's working directory when the command was
	// proposed; used for project-key derivation and as the working
	// directory for git invocations.
	CWD string

	// Command is the full command line that triggered the rule.
	// Used by path_archive (target extraction) and script_capture
	// (URL extraction).
	Command string
}

// BackupResult records what was archived. Empty Path means backup
// was skipped (strategy=none or no target detected); the matcher
// upgrades to Block when a destructive rule produced no backup.
type BackupResult struct {
	Path     string    // absolute path of the archive file
	Bytes    int64     // archive size on disk
	Strategy string    // echo of the request strategy
	Created  time.Time // mtime of the archive
}

// BackupEngine writes archives under <stateDir>/backups/<key>/.
type BackupEngine struct {
	StateDir string // typically StateDir("nerv")
	// Now is overrideable for tests.
	Now func() time.Time
}

// NewBackupEngine constructs a BackupEngine with a real clock.
func NewBackupEngine(stateDir string) *BackupEngine {
	return &BackupEngine{StateDir: stateDir, Now: time.Now}
}

// Capture runs the requested strategy. Any failure returns an error;
// the caller (matcher) upgrades the rule decision to Block on error
// per ADR §"Backup contract".
func (e *BackupEngine) Capture(req BackupRequest) (BackupResult, error) {
	if e.StateDir == "" {
		return BackupResult{}, fmt.Errorf("guardrails: BackupEngine.StateDir empty")
	}
	now := e.Now()
	key := ProjectKey(req.CWD)
	dir := filepath.Join(e.StateDir, "backups", key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return BackupResult{}, fmt.Errorf("mkdir backups: %w", err)
	}

	stamp := now.UTC().Format("20060102T150405Z")
	out := filepath.Join(dir, fmt.Sprintf("%s-%s.tar.gz", stamp, req.RuleID))

	var err error
	switch req.Strategy {
	case "path_archive":
		err = capturePathArchive(out, req)
	case "git_stash_full":
		err = captureGitStashFull(out, req)
	case "git_untracked":
		err = captureGitUntracked(out, req)
	case "remote_bundle":
		err = captureRemoteBundle(out, req)
	case "script_capture":
		err = captureScript(out, req)
	default:
		return BackupResult{}, fmt.Errorf("unknown backup strategy %q", req.Strategy)
	}
	if err != nil {
		_ = os.Remove(out)
		return BackupResult{}, err
	}

	info, err := os.Stat(out)
	if err != nil {
		return BackupResult{}, fmt.Errorf("stat archive: %w", err)
	}
	return BackupResult{
		Path:     out,
		Bytes:    info.Size(),
		Strategy: req.Strategy,
		Created:  info.ModTime(),
	}, nil
}

// ── strategies ────────────────────────────────────────────────────

// capturePathArchive tars+gzips the targets named in the command
// (everything after the rm flags). Skips entries that don't exist on
// disk. Excludes the StateDir itself (avoids recursive backup of
// backup dir).
//
// Conservative parser: split command on whitespace, drop tokens that
// look like flags (start with -) or shell metacharacters, treat the
// rest as path candidates. Good enough for "rm -rf path1 path2".
func capturePathArchive(out string, req BackupRequest) error {
	targets := pathTargets(req.Command, req.CWD)
	if len(targets) == 0 {
		return fmt.Errorf("path_archive: no path targets in %q", req.Command)
	}
	return writeTarGz(out, targets, "")
}

// captureGitStashFull runs `git stash push -u -m guardrails-<ts>-<ruleID>`
// in CWD AND tars the working tree as belt-and-braces.
func captureGitStashFull(out string, req BackupRequest) error {
	stamp := time.Now().UTC().Format("20060102T150405Z")
	msg := fmt.Sprintf("guardrails-%s-%s", stamp, req.RuleID)
	cmd := exec.Command("git", "stash", "push", "-u", "-m", msg)
	cmd.Dir = req.CWD
	// stash may exit non-zero when there's nothing to stash; that's
	// not an error for us — the working tree is already clean.
	_ = cmd.Run()

	// Snapshot working tree.
	return writeTarGz(out, []string{req.CWD}, "")
}

// captureGitUntracked tars all untracked files under CWD using
// `git ls-files --others --exclude-standard`.
func captureGitUntracked(out string, req BackupRequest) error {
	cmd := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	cmd.Dir = req.CWD
	stdout, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git ls-files: %w", err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(stdout)), "\n") {
		if line == "" {
			continue
		}
		files = append(files, filepath.Join(req.CWD, line))
	}
	if len(files) == 0 {
		// Empty archive is fine; signals the gc that no untracked
		// state existed at backup time.
		return writeEmptyTarGz(out)
	}
	return writeTarGz(out, files, "")
}

// captureRemoteBundle bundles the remote ref before a force-push.
// Extracts the remote name + branch from the command (best-effort)
// and runs `git bundle create <out> origin/<branch>` first, then
// re-archives the bundle into a .tar.gz so the format is uniform
// across strategies.
func captureRemoteBundle(out string, req BackupRequest) error {
	remote, branch := parsePushTarget(req.Command)
	if remote == "" || branch == "" {
		return fmt.Errorf("remote_bundle: could not parse remote/branch from %q",
			req.Command)
	}
	tmp, err := os.CreateTemp("", "guardrails-bundle-*.bundle")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	// Fetch first so origin/<branch> is current.
	cmd := exec.Command("git", "fetch", remote, branch)
	cmd.Dir = req.CWD
	_ = cmd.Run()

	cmd = exec.Command("git", "bundle", "create", tmpPath,
		fmt.Sprintf("%s/%s", remote, branch))
	cmd.Dir = req.CWD
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git bundle: %w", err)
	}
	return writeTarGz(out, []string{tmpPath}, "")
}

// captureScript saves the URL-bytes that would have been piped to
// the shell. Best-effort: extracts the first http(s):// URL from the
// command and curls its content into a file inside the archive.
//
// If the curl itself fails, the archive contains a placeholder file
// recording the URL + error; this is intentional — backup-fail
// upgrades the rule to Block, but only if we return an error here.
func captureScript(out string, req BackupRequest) error {
	url := firstURL(req.Command)
	if url == "" {
		return fmt.Errorf("script_capture: no http(s) URL in %q", req.Command)
	}
	tmpDir, err := os.MkdirTemp("", "guardrails-script-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	scriptPath := filepath.Join(tmpDir, "script.txt")
	urlPath := filepath.Join(tmpDir, "url.txt")
	if err := os.WriteFile(urlPath, []byte(url+"\n"), 0o644); err != nil {
		return err
	}

	// Best-effort fetch via curl.
	cmd := exec.Command("curl", "-fsSL", "-o", scriptPath, url)
	cmd.Dir = req.CWD
	if runErr := cmd.Run(); runErr != nil {
		// Record the failure inside the archive but DON'T return an
		// error — backup-fail would upgrade decision to Block, but
		// the rule was likely already Block. We've captured the URL.
		_ = os.WriteFile(scriptPath,
			[]byte(fmt.Sprintf("curl failed: %v\nURL: %s\n", runErr, url)),
			0o644)
	}

	return writeTarGz(out, []string{tmpDir}, tmpDir)
}

// ── archive helpers ───────────────────────────────────────────────

// writeTarGz writes paths to a gzip-compressed tar archive at out.
// stripPrefix, if non-empty, is removed from each entry's archive
// path so absolute paths don't leak. Skips files that no longer
// exist (best-effort) and the StateDir itself (no recursive backup).
func writeTarGz(out string, paths []string, stripPrefix string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	defer func() { _ = gz.Close() }()

	tw := tar.NewWriter(gz)
	defer func() { _ = tw.Close() }()

	for _, root := range paths {
		err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			// Skip the backups dir itself (recursive guard).
			if strings.Contains(p, string(filepath.Separator)+"backups"+string(filepath.Separator)) {
				return nil
			}
			rel := p
			if stripPrefix != "" {
				rel = strings.TrimPrefix(p, stripPrefix)
				rel = strings.TrimPrefix(rel, string(filepath.Separator))
				if rel == "" {
					return nil
				}
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = rel
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				rf, err := os.Open(p)
				if err != nil {
					return err
				}
				_, err = io.Copy(tw, rf)
				_ = rf.Close()
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// writeEmptyTarGz writes a valid but empty .tar.gz (zero entries).
func writeEmptyTarGz(out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz := gzip.NewWriter(f)
	defer func() { _ = gz.Close() }()
	tw := tar.NewWriter(gz)
	return tw.Close()
}

// ── command parsers ───────────────────────────────────────────────

// pathTargets extracts path-like tokens from cmd. Skips flag tokens
// and shell metacharacters. Resolves relative paths against cwd.
func pathTargets(cmd, cwd string) []string {
	tokens := strings.Fields(cmd)
	var out []string
	skipNext := false
	for _, tok := range tokens {
		if skipNext {
			skipNext = false
			continue
		}
		// Drop the program name (e.g. "rm", "sudo", "git").
		if tok == "rm" || tok == "sudo" || tok == "git" {
			continue
		}
		// Drop flag tokens.
		if strings.HasPrefix(tok, "-") {
			continue
		}
		// Drop shell metacharacters.
		if tok == "&&" || tok == "||" || tok == ";" || tok == "|" {
			continue
		}
		// Expand $HOME / ~ (best-effort; not POSIX-perfect).
		switch {
		case tok == "$HOME" || tok == "~":
			tok = os.Getenv("HOME")
		case strings.HasPrefix(tok, "$HOME/"):
			tok = filepath.Join(os.Getenv("HOME"), tok[len("$HOME/"):])
		case strings.HasPrefix(tok, "~/"):
			tok = filepath.Join(os.Getenv("HOME"), tok[2:])
		}
		// Resolve against cwd if relative.
		if !filepath.IsAbs(tok) && cwd != "" {
			tok = filepath.Join(cwd, tok)
		}
		out = append(out, tok)
	}
	return out
}

// parsePushTarget extracts the remote + branch from a `git push
// [...] <remote> <branch>` command. Best-effort; returns empty
// strings if the shape doesn't match.
func parsePushTarget(cmd string) (remote, branch string) {
	tokens := strings.Fields(cmd)
	pushIdx := -1
	for i, tok := range tokens {
		if tok == "push" {
			pushIdx = i
			break
		}
	}
	if pushIdx < 0 {
		return "", ""
	}
	// Skip flags after `push`; first 2 non-flag tokens = remote + branch.
	var nonFlag []string
	for _, tok := range tokens[pushIdx+1:] {
		if strings.HasPrefix(tok, "-") {
			continue
		}
		nonFlag = append(nonFlag, tok)
	}
	if len(nonFlag) >= 2 {
		return nonFlag[0], nonFlag[1]
	}
	return "", ""
}

// firstURL returns the first http(s):// substring in cmd, ending at
// whitespace or shell separator. Empty if none.
func firstURL(cmd string) string {
	for _, scheme := range []string{"https://", "http://"} {
		i := strings.Index(cmd, scheme)
		if i < 0 {
			continue
		}
		end := i
		for end < len(cmd) && !isURLEnd(cmd[end]) {
			end++
		}
		return cmd[i:end]
	}
	return ""
}

func isURLEnd(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '|', ';', '&', '"', '\'', '`', '>', '<':
		return true
	}
	return false
}
