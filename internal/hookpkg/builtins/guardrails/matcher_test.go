package guardrails

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// rulepacksDir is the canonical location loaded from the in-tree
// spec/. Tests resolve repo root by walking up from this file's cwd.
func rulepacksDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "spec", "rulepacks")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod from %q", dir)
		}
		dir = parent
	}
}

// envFor builds a minimal PreToolUse envelope carrying a Bash
// command, suitable for matcher tests.
func envFor(t *testing.T, cmd string) *envelope.Envelope {
	t.Helper()
	pl, err := json.Marshal(map[string]any{
		"tool_name": "Bash",
		"tool_input": map[string]any{
			"command": cmd,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &envelope.Envelope{
		Event:   "PreToolUse",
		Payload: pl,
	}
}

// loadCorpus reads testdata/<pack>/match.txt or no-match.txt. Each
// non-blank, non-comment line is one entry. match.txt entries are
// "<rule-id>\t<command>"; no-match.txt entries are bare commands.
func loadCorpus(t *testing.T, path string, hasRuleID bool) [][2]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open corpus %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var out [][2]string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if hasRuleID {
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 {
				t.Fatalf("malformed match.txt line: %q", line)
			}
			out = append(out, [2]string{parts[0], parts[1]})
		} else {
			out = append(out, [2]string{"", line})
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	return out
}

func TestLoadPacks(t *testing.T) {
	packs, err := LoadPacks(rulepacksDir(t))
	if err != nil {
		t.Fatalf("LoadPacks: %v", err)
	}
	if len(packs) == 0 {
		t.Fatal("LoadPacks returned no packs")
	}
	// Spot-check git pack.
	var git *Pack
	for i := range packs {
		if packs[i].Name == "git" {
			git = &packs[i]
			break
		}
	}
	if git == nil {
		t.Fatal("git pack not found")
	}
	if got := len(git.Rules); got != 8 {
		t.Errorf("git pack rule count = %d, want 8", got)
	}
	for _, r := range git.Rules {
		if r.compiled == nil {
			t.Errorf("rule %s: regex not compiled", r.ID)
		}
	}
}

// allPacks names every rulepack with a corpus directory under
// testdata/. Tests iterate over this list so adding a new pack
// requires only a corpus dir, not a test edit.
var allPacks = []string{"git", "rm", "gh", "curl", "wget", "sudo"}

func TestMatcher_PositiveCorpus(t *testing.T) {
	packs, err := LoadPacks(rulepacksDir(t))
	if err != nil {
		t.Fatalf("LoadPacks: %v", err)
	}
	m := NewMatcher()
	for _, pack := range allPacks {
		t.Run(pack, func(t *testing.T) {
			corpus := loadCorpus(t,
				filepath.Join("testdata", pack, "match.txt"), true)
			for _, e := range corpus {
				wantRule, cmd := e[0], e[1]
				out := m.Evaluate(envFor(t, cmd), packs, nil)
				if out.RuleID != wantRule {
					t.Errorf("cmd %q: got rule %q, want %q (action=%s)",
						cmd, out.RuleID, wantRule, out.Action)
				}
			}
		})
	}
}

func TestMatcher_NegativeCorpus(t *testing.T) {
	packs, err := LoadPacks(rulepacksDir(t))
	if err != nil {
		t.Fatalf("LoadPacks: %v", err)
	}
	m := NewMatcher()
	for _, pack := range allPacks {
		t.Run(pack, func(t *testing.T) {
			corpus := loadCorpus(t,
				filepath.Join("testdata", pack, "no-match.txt"), false)
			for _, e := range corpus {
				cmd := e[1]
				out := m.Evaluate(envFor(t, cmd), packs, nil)
				if out.RuleID != "" {
					t.Errorf("cmd %q: matched rule %q (want no match)",
						cmd, out.RuleID)
				}
				if out.Action != merger.Allow {
					t.Errorf("cmd %q: action=%s, want Allow",
						cmd, out.Action)
				}
			}
		})
	}
}

func TestMatcher_RewriteAppliesForcePush(t *testing.T) {
	packs, err := LoadPacks(rulepacksDir(t))
	if err != nil {
		t.Fatalf("LoadPacks: %v", err)
	}
	m := NewMatcher()
	out := m.Evaluate(envFor(t, "git push --force origin feat/x"), packs, nil)
	if out.RuleID != "git-push-force" {
		t.Fatalf("rule = %q, want git-push-force", out.RuleID)
	}
	if out.Action != merger.Rewrite {
		t.Fatalf("action = %s, want Rewrite", out.Action)
	}
	want := "git push --force-with-lease origin feat/x"
	if out.RewrittenCommand != want {
		t.Errorf("rewritten = %q, want %q", out.RewrittenCommand, want)
	}
}

func TestMatcher_BlockNoVerify(t *testing.T) {
	packs, err := LoadPacks(rulepacksDir(t))
	if err != nil {
		t.Fatalf("LoadPacks: %v", err)
	}
	m := NewMatcher()
	out := m.Evaluate(envFor(t, "git commit --no-verify -m wip"), packs, nil)
	if out.RuleID != "git-no-verify" {
		t.Fatalf("rule = %q, want git-no-verify", out.RuleID)
	}
	if out.Action != merger.Block {
		t.Errorf("action = %s, want Block", out.Action)
	}
	if !strings.Contains(out.Reason, "--no-verify") {
		t.Errorf("reason missing --no-verify mention: %q", out.Reason)
	}
}

func TestMatcher_OverrideMode(t *testing.T) {
	packs, err := LoadPacks(rulepacksDir(t))
	if err != nil {
		t.Fatalf("LoadPacks: %v", err)
	}
	m := NewMatcher()

	// Override git-no-verify default 'block' → 'allow'.
	overrides := map[string]string{"git-no-verify": "allow"}
	out := m.Evaluate(envFor(t, "git commit --no-verify -m x"), packs, overrides)
	if out.Action != merger.Allow {
		t.Errorf("override allow: action = %s, want Allow", out.Action)
	}

	// Override git-no-verify → 'rewrite' should strip the flag.
	overrides = map[string]string{"git-no-verify": "rewrite"}
	out = m.Evaluate(envFor(t, "git commit --no-verify -m x"), packs, overrides)
	if out.Action != merger.Rewrite {
		t.Errorf("override rewrite: action = %s, want Rewrite", out.Action)
	}
	if out.RewrittenCommand != "git commit -m x" {
		t.Errorf("rewritten = %q, want %q", out.RewrittenCommand, "git commit -m x")
	}
}

func TestMatcher_NonBashIgnored(t *testing.T) {
	packs, err := LoadPacks(rulepacksDir(t))
	if err != nil {
		t.Fatalf("LoadPacks: %v", err)
	}
	m := NewMatcher()
	pl, _ := json.Marshal(map[string]any{
		"tool_name":  "Edit",
		"tool_input": map[string]any{"command": "git commit --no-verify"},
	})
	env := &envelope.Envelope{Event: "PreToolUse", Payload: pl}
	out := m.Evaluate(env, packs, nil)
	if out.Action != merger.Allow || out.RuleID != "" {
		t.Errorf("Edit-tool envelope: rule=%q action=%s, want allow + no rule",
			out.RuleID, out.Action)
	}
}

func TestMatcher_NonPreToolUseIgnored(t *testing.T) {
	packs, err := LoadPacks(rulepacksDir(t))
	if err != nil {
		t.Fatalf("LoadPacks: %v", err)
	}
	m := NewMatcher()
	pl, _ := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "git commit --no-verify"},
	})
	env := &envelope.Envelope{Event: "PostToolUse", Payload: pl}
	out := m.Evaluate(env, packs, nil)
	if out.Action != merger.Allow {
		t.Errorf("PostToolUse envelope: action=%s, want Allow", out.Action)
	}
}

func TestMatcher_DestructiveFlagsPropagate(t *testing.T) {
	packs, err := LoadPacks(rulepacksDir(t))
	if err != nil {
		t.Fatalf("LoadPacks: %v", err)
	}
	m := NewMatcher()
	out := m.Evaluate(envFor(t, "git reset --hard HEAD"), packs, nil)
	if out.RuleID != "git-reset-hard" {
		t.Fatalf("rule = %q, want git-reset-hard", out.RuleID)
	}
	if !out.Destructive {
		t.Error("Destructive flag not propagated")
	}
	if out.BackupStrategy != "git_stash_full" {
		t.Errorf("BackupStrategy = %q, want git_stash_full", out.BackupStrategy)
	}
}
