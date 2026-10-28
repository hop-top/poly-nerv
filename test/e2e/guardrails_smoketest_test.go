package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/hookpkg/builtins"
	guardrails "hop.top/nerv/internal/hookpkg/builtins/guardrails"
	"hop.top/nerv/internal/merger"
)

// envForBash builds a PreToolUse envelope carrying a Bash command
// + cwd in session info.
func envForBash(cmd, cwd string) *envelope.Envelope {
	pl, _ := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": cmd},
	})
	return &envelope.Envelope{
		Event:   "PreToolUse",
		Payload: pl,
		Session: envelope.SessionInfo{CWD: cwd},
	}
}

// repoRulepacksDir resolves spec/rulepacks/ from the test cwd.
// e2e tests run from test/e2e/ so we walk up to find go.mod.
func repoRulepacksDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "spec", "rulepacks")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// TestGuardrailsSmoke_BlocksRmRfHome verifies the highest-blast
// destructive rule is blocked end-to-end (registration + matcher +
// reason surfaced).
func TestGuardrailsSmoke_BlocksRmRfHome(t *testing.T) {
	t.Setenv("NERV_RULEPACKS_DIR", repoRulepacksDir(t))

	def, ok := builtins.ByName("guardrails")
	if !ok {
		t.Fatal("guardrails not registered")
	}

	env := envForBash("rm -rf $HOME", t.TempDir())
	d, err := def.Func(env, builtins.BuiltinRuntimeCfg{
		StateDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if d.Action != merger.Block {
		t.Errorf("Action = %s, want Block", d.Action)
	}
	if d.Message == "" {
		t.Error("Block decision missing reason message")
	}
	if rule, _ := d.Metadata["guardrails_rule"].(string); rule != "rm-rf-home" {
		t.Errorf("Metadata.guardrails_rule = %q, want rm-rf-home", rule)
	}
}

// TestGuardrailsSmoke_RewritesForcePush verifies a non-protected
// branch force-push is rewritten end-to-end and the new command is
// in the Decision payload.
func TestGuardrailsSmoke_RewritesForcePush(t *testing.T) {
	t.Setenv("NERV_RULEPACKS_DIR", repoRulepacksDir(t))

	def, _ := builtins.ByName("guardrails")
	env := envForBash("git push --force origin user/jadb/x", t.TempDir())
	d, err := def.Func(env, builtins.BuiltinRuntimeCfg{StateDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if d.Action != merger.Rewrite {
		t.Fatalf("Action = %s, want Rewrite", d.Action)
	}
	rw, ok := d.Rewrite.(map[string]any)
	if !ok {
		t.Fatalf("Rewrite payload type = %T", d.Rewrite)
	}
	cmd, _ := rw["command"].(string)
	want := "git push --force-with-lease origin user/jadb/x"
	if cmd != want {
		t.Errorf("rewritten = %q, want %q", cmd, want)
	}
}

// TestGuardrailsSmoke_BackupBeforeDestructiveAllow verifies the
// destructive-rule + override-to-allow path captures a backup
// archive on disk before the Allow decision is returned.
func TestGuardrailsSmoke_BackupBeforeDestructiveAllow(t *testing.T) {
	t.Setenv("NERV_RULEPACKS_DIR", repoRulepacksDir(t))

	def, _ := builtins.ByName("guardrails")

	// Set up a cwd with content for path_archive to capture.
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "scratch.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	state := t.TempDir()
	env := envForBash("rm -rf *", cwd)
	d, err := def.Func(env, builtins.BuiltinRuntimeCfg{
		StateDir: state,
		UserCfg: map[string]any{
			"rules": map[string]any{
				"rm-rf-glob": "allow", // override hitl → allow
			},
		},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if d.Action != merger.Allow {
		t.Errorf("Action = %s, want Allow (override applied)", d.Action)
	}

	backupDir := filepath.Join(state, "backups", guardrails.ProjectKey(cwd))
	entries, err := os.ReadDir(backupDir)
	if err != nil || len(entries) == 0 {
		t.Errorf("expected backup archive in %s, err=%v entries=%d",
			backupDir, err, len(entries))
	}
}

// TestGuardrailsSmoke_AllowsBenign verifies non-matching commands
// pass through with no rule fired.
func TestGuardrailsSmoke_AllowsBenign(t *testing.T) {
	t.Setenv("NERV_RULEPACKS_DIR", repoRulepacksDir(t))

	def, _ := builtins.ByName("guardrails")
	for _, cmd := range []string{
		"ls -la",
		"git status",
		"git commit -m feat",
		"go test ./...",
		"echo hello",
	} {
		env := envForBash(cmd, t.TempDir())
		d, err := def.Func(env, builtins.BuiltinRuntimeCfg{StateDir: t.TempDir()})
		if err != nil {
			t.Errorf("cmd %q: Handle err = %v", cmd, err)
			continue
		}
		if d.Action != merger.Allow {
			t.Errorf("cmd %q: Action = %s, want Allow", cmd, d.Action)
		}
	}
}

// TestGuardrailsSmoke_DisabledShortCircuits verifies enabled=false
// skips evaluation entirely (no backup, no rule lookup).
func TestGuardrailsSmoke_DisabledShortCircuits(t *testing.T) {
	t.Setenv("NERV_RULEPACKS_DIR", repoRulepacksDir(t))

	def, _ := builtins.ByName("guardrails")
	env := envForBash("rm -rf /", t.TempDir())
	d, err := def.Func(env, builtins.BuiltinRuntimeCfg{
		StateDir: t.TempDir(),
		UserCfg:  map[string]any{"enabled": false},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if d.Action != merger.Allow {
		t.Errorf("disabled+rm-rf-/: Action = %s, want Allow", d.Action)
	}
}
