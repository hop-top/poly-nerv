package guardrails

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/hookpkg/builtins"
	"hop.top/nerv/internal/merger"
)

func TestRegistration(t *testing.T) {
	def, ok := builtins.ByName("guardrails")
	if !ok {
		t.Fatal("guardrails not registered in builtins.All")
	}
	if def.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", def.SchemaVersion)
	}
	if len(def.Events) != 1 || def.Events[0] != "PreToolUse" {
		t.Errorf("Events = %v, want [PreToolUse]", def.Events)
	}
}

// TestHandle_DefaultEnabledBlocksRm verifies the default-on path:
// even with no user config, Handle blocks 'rm -rf /'.
func TestHandle_DefaultEnabledBlocksRm(t *testing.T) {
	// Point loadCachedPacks at the in-tree spec/rulepacks/.
	dir := rulepacksDir(t)
	t.Setenv("NERV_RULEPACKS_DIR", dir)
	resetPackCache()

	pl, _ := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "rm -rf /"},
	})
	env := &envelope.Envelope{Event: "PreToolUse", Payload: pl}

	state := t.TempDir()
	d, err := Handle(env, builtins.BuiltinRuntimeCfg{
		StateDir: state,
		UserCfg:  nil, // default-on
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if d.Action != merger.Block {
		t.Errorf("Action = %s, want Block", d.Action)
	}
}

// TestHandle_DisabledAllowsAll verifies enabled=false short-circuits.
func TestHandle_DisabledAllowsAll(t *testing.T) {
	pl, _ := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "rm -rf /"},
	})
	env := &envelope.Envelope{Event: "PreToolUse", Payload: pl}

	d, err := Handle(env, builtins.BuiltinRuntimeCfg{
		StateDir: t.TempDir(),
		UserCfg:  map[string]any{"enabled": false},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if d.Action != merger.Allow {
		t.Errorf("Action = %s, want Allow", d.Action)
	}
}

// TestHandle_RewriteCarriesNewCommand verifies rewrite mode produces
// a Decision.Rewrite payload with the rewritten command.
func TestHandle_RewriteCarriesNewCommand(t *testing.T) {
	dir := rulepacksDir(t)
	t.Setenv("NERV_RULEPACKS_DIR", dir)
	resetPackCache()

	pl, _ := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "git push --force origin user/jadb/x"},
	})
	env := &envelope.Envelope{Event: "PreToolUse", Payload: pl}

	d, err := Handle(env, builtins.BuiltinRuntimeCfg{StateDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if d.Action != merger.Rewrite {
		t.Fatalf("Action = %s, want Rewrite", d.Action)
	}
	rw, ok := d.Rewrite.(map[string]any)
	if !ok {
		t.Fatalf("Rewrite payload type = %T, want map[string]any", d.Rewrite)
	}
	cmd, _ := rw["command"].(string)
	want := "git push --force-with-lease origin user/jadb/x"
	if cmd != want {
		t.Errorf("rewritten = %q, want %q", cmd, want)
	}
}

// TestHandle_DestructiveCapturesBackup verifies the backup contract:
// hitl-mode rules with destructive=true (e.g. git-reset-hard) trigger
// a backup archive on disk before the decision flows out.
//
// Note: Outcome.Action for hitl falls back to Block in v1, so this
// test verifies the path is plumbed even when the action is Block.
// Once hitl ships, the rule will resolve to Allow (after user
// approval) and the same code path captures the archive.
func TestHandle_DestructiveCapturesBackup(t *testing.T) {
	dir := rulepacksDir(t)
	t.Setenv("NERV_RULEPACKS_DIR", dir)
	resetPackCache()

	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	pl, _ := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "rm -rf *"},
	})
	env := &envelope.Envelope{
		Event:   "PreToolUse",
		Payload: pl,
		Session: envelope.SessionInfo{CWD: cwd},
	}

	state := t.TempDir()
	// Override default mode for rm-rf-glob: hitl → allow so backup runs.
	d, err := Handle(env, builtins.BuiltinRuntimeCfg{
		StateDir: state,
		UserCfg: map[string]any{
			"rules": map[string]any{
				"rm-rf-glob": "allow",
			},
		},
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if d.Action != merger.Allow {
		t.Errorf("Action = %s, want Allow (override rule applied)", d.Action)
	}
	// Backup is only triggered when destructive AND not Block. Allow
	// + destructive should have produced an archive in
	// state/backups/<key>/.
	backupRoot := filepath.Join(state, "backups", ProjectKey(cwd))
	entries, err := os.ReadDir(backupRoot)
	if err != nil || len(entries) == 0 {
		t.Errorf("expected backup archive under %q, err=%v entries=%d",
			backupRoot, err, len(entries))
	}
}

// resetPackCache clears the global cache so successive tests can
// re-load via the env-var override.
func resetPackCache() {
	packCache.once = sync.Once{}
	packCache.packs = nil
	packCache.err = nil
}
