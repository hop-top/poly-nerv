package config

import (
	"os"
	"path/filepath"
	"testing"

	nerverr "hop.top/nerv/internal/errors"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_SingleFile(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
version: "1.0-draft"
defaults:
  timeout: 5
  on_error: abort
hooks:
  - name: lint
    on: PreToolUse
    run: ./lint.sh
`)
	cfg, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != "1.0-draft" {
		t.Errorf("version = %q, want 1.0-draft", cfg.Version)
	}
	if cfg.Defaults.Timeout != 5 {
		t.Errorf("timeout = %d, want 5", cfg.Defaults.Timeout)
	}
	if cfg.Defaults.OnError != "abort" {
		t.Errorf("on_error = %q, want abort", cfg.Defaults.OnError)
	}
	if len(cfg.Hooks) != 1 {
		t.Fatalf("hooks len = %d, want 1", len(cfg.Hooks))
	}
	if cfg.Hooks[0].Name != "lint" {
		t.Errorf("hook name = %q, want lint", cfg.Hooks[0].Name)
	}
}

func TestLoad_MergeOrder(t *testing.T) {
	tmp := t.TempDir()

	system := filepath.Join(tmp, "system")
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")

	writeFile(t, system, "hooks.yaml", `
version: "0.9"
defaults:
  timeout: 20
hooks:
  - name: system-hook
    on: PreToolUse
    run: ./system.sh
`)
	writeFile(t, user, "hooks.yaml", `
version: "1.0-draft"
hooks:
  - name: user-hook
    on: PostToolUse
    run: ./user.sh
`)
	writeFile(t, project, "hooks.yaml", `
defaults:
  timeout: 3
  on_error: abort
hooks:
  - name: project-hook
    on:
      - PreToolUse
      - PostToolUse
    run: ./project.sh
`)

	paths := []string{
		filepath.Join(system, "hooks.yaml"),
		filepath.Join(user, "hooks.yaml"),
		filepath.Join(project, "hooks.yaml"),
	}

	cfg, err := loadFromPaths(paths)
	if err != nil {
		t.Fatal(err)
	}

	// Last version wins.
	if cfg.Version != "1.0-draft" {
		t.Errorf("version = %q, want 1.0-draft", cfg.Version)
	}
	// Project timeout overrides system.
	if cfg.Defaults.Timeout != 3 {
		t.Errorf("timeout = %d, want 3", cfg.Defaults.Timeout)
	}
	// Project on_error overrides default.
	if cfg.Defaults.OnError != "abort" {
		t.Errorf("on_error = %q, want abort", cfg.Defaults.OnError)
	}
	// All hooks appended.
	if len(cfg.Hooks) != 3 {
		t.Fatalf("hooks len = %d, want 3", len(cfg.Hooks))
	}
	if cfg.Hooks[0].Name != "system-hook" {
		t.Errorf("first hook = %q, want system-hook", cfg.Hooks[0].Name)
	}
	if cfg.Hooks[2].Name != "project-hook" {
		t.Errorf("last hook = %q, want project-hook", cfg.Hooks[2].Name)
	}
}

func TestLoad_BadYAML(t *testing.T) {
	tmp := t.TempDir()

	good := filepath.Join(tmp, "good")
	bad := filepath.Join(tmp, "bad")

	writeFile(t, bad, "hooks.yaml", `{{{not yaml`)
	writeFile(t, good, "hooks.yaml", `
version: "1.0-draft"
hooks:
  - name: ok
    on: PreToolUse
    run: ./ok.sh
`)

	paths := []string{
		filepath.Join(bad, "hooks.yaml"),
		filepath.Join(good, "hooks.yaml"),
	}

	cfg, err := loadFromPaths(paths)
	if err == nil {
		t.Fatal("expected ErrConfig, got nil")
	}
	if !nerverr.Is(err, nerverr.ErrConfig) {
		t.Errorf("error category = %T, want ErrConfig", err)
	}
	// Good layer still loaded despite bad layer.
	if len(cfg.Hooks) != 1 {
		t.Fatalf("hooks len = %d, want 1 (good layer)", len(cfg.Hooks))
	}
}

func TestLoad_MissingFile(t *testing.T) {
	cfg, err := loadFromPaths([]string{"/nonexistent/hooks.yaml"})
	if err != nil {
		t.Fatalf("missing file should not error, got %v", err)
	}
	if len(cfg.Hooks) != 0 {
		t.Errorf("hooks len = %d, want 0", len(cfg.Hooks))
	}
}

func TestLoad_Defaults(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
version: "1.0-draft"
hooks: []
`)
	cfg, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Timeout != defaultTimeout {
		t.Errorf("default timeout = %d, want %d", cfg.Defaults.Timeout, defaultTimeout)
	}
	if cfg.Defaults.OnError != "continue" {
		t.Errorf("default on_error = %q, want continue", cfg.Defaults.OnError)
	}
	if cfg.Defaults.Parallel != nil && *cfg.Defaults.Parallel {
		t.Error("default parallel should be nil or false")
	}
}

func TestLoad_BusRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
version: "1.0-draft"
hooks: []
bus:
  adapter: jsonl
  sinks:
    - type: jsonl
      path: /var/log/nerv/events.jsonl
      rotate:
        max_bytes: 10485760
        max_files: 5
    - type: stdout
  network:
    endpoint: tcp://127.0.0.1:9000
    auth: token-abc
  filters:
    - event: inbox_message_added
      include: ["urgent", "review"]
      exclude: ["debug"]
`)
	cfg, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bus.Adapter != "jsonl" {
		t.Errorf("adapter = %q, want jsonl", cfg.Bus.Adapter)
	}
	if len(cfg.Bus.Sinks) != 2 {
		t.Fatalf("sinks len = %d, want 2", len(cfg.Bus.Sinks))
	}
	s0 := cfg.Bus.Sinks[0]
	if s0.Type != "jsonl" || s0.Path != "/var/log/nerv/events.jsonl" {
		t.Errorf("sink[0] = %+v", s0)
	}
	if s0.Rotate.MaxBytes != 10485760 || s0.Rotate.MaxFiles != 5 {
		t.Errorf("sink[0].rotate = %+v", s0.Rotate)
	}
	if cfg.Bus.Sinks[1].Type != "stdout" {
		t.Errorf("sink[1].type = %q, want stdout", cfg.Bus.Sinks[1].Type)
	}
	if cfg.Bus.Network.Endpoint != "tcp://127.0.0.1:9000" {
		t.Errorf("network.endpoint = %q", cfg.Bus.Network.Endpoint)
	}
	if cfg.Bus.Network.Auth != "token-abc" {
		t.Errorf("network.auth = %q", cfg.Bus.Network.Auth)
	}
	if len(cfg.Bus.Filters) != 1 {
		t.Fatalf("filters len = %d, want 1", len(cfg.Bus.Filters))
	}
	f := cfg.Bus.Filters[0]
	if f.Event != "inbox_message_added" {
		t.Errorf("filter[0].event = %q", f.Event)
	}
	if len(f.Include) != 2 || f.Include[0] != "urgent" {
		t.Errorf("filter[0].include = %v", f.Include)
	}
	if len(f.Exclude) != 1 || f.Exclude[0] != "debug" {
		t.Errorf("filter[0].exclude = %v", f.Exclude)
	}
}

func TestLoad_BusDefaultAdapter(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
version: "1.0-draft"
hooks: []
`)
	cfg, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bus.Adapter != "memory" {
		t.Errorf("default adapter = %q, want memory", cfg.Bus.Adapter)
	}
	if len(cfg.Bus.Sinks) != 0 {
		t.Errorf("default sinks len = %d, want 0", len(cfg.Bus.Sinks))
	}
	if cfg.Bus.Network.Endpoint != "" {
		t.Errorf("default network.endpoint = %q, want \"\"", cfg.Bus.Network.Endpoint)
	}
	if len(cfg.Bus.Filters) != 0 {
		t.Errorf("default filters len = %d, want 0", len(cfg.Bus.Filters))
	}
}

func TestLoad_BusInvalidAdapter(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
bus:
  adapter: pigeon
`)
	_, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err == nil {
		t.Fatal("expected ErrConfig for invalid adapter, got nil")
	}
	if !nerverr.Is(err, nerverr.ErrConfig) {
		t.Errorf("error category = %T, want ErrConfig", err)
	}
}

func TestLoad_BusNetworkRequiresEndpoint(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
bus:
  adapter: network
`)
	_, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err == nil {
		t.Fatal("expected ErrConfig for missing endpoint, got nil")
	}
	if !nerverr.Is(err, nerverr.ErrConfig) {
		t.Errorf("error category = %T, want ErrConfig", err)
	}
}

func TestLoad_BusSinkPathMustBeAbsolute(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
bus:
  adapter: memory
  sinks:
    - type: jsonl
      path: relative/path.jsonl
`)
	_, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err == nil {
		t.Fatal("expected ErrConfig for non-absolute path, got nil")
	}
	if !nerverr.Is(err, nerverr.ErrConfig) {
		t.Errorf("error category = %T, want ErrConfig", err)
	}
}

func TestLoad_BusInvalidSinkType(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
bus:
  adapter: memory
  sinks:
    - type: carrier-pigeon
`)
	_, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err == nil {
		t.Fatal("expected ErrConfig for invalid sink type, got nil")
	}
	if !nerverr.Is(err, nerverr.ErrConfig) {
		t.Errorf("error category = %T, want ErrConfig", err)
	}
}

func TestLoad_BusSinkJsonlRequiresPath(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
bus:
  adapter: memory
  sinks:
    - type: jsonl
`)
	_, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err == nil {
		t.Fatal("expected ErrConfig for jsonl without path, got nil")
	}
	if !nerverr.Is(err, nerverr.ErrConfig) {
		t.Errorf("error category = %T, want ErrConfig", err)
	}
}

func TestLoad_BusNetworkSinkRequiresEndpoint(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
bus:
  adapter: memory
  sinks:
    - type: network
`)
	_, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err == nil {
		t.Fatal("expected ErrConfig for network sink without endpoint, got nil")
	}
}

func TestLoad_BusLayeredMerge(t *testing.T) {
	tmp := t.TempDir()
	system := filepath.Join(tmp, "system")
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")

	// system: jsonl adapter + one sink
	writeFile(t, system, "hooks.yaml", `
bus:
  adapter: jsonl
  sinks:
    - type: jsonl
      path: /var/log/nerv/system.jsonl
  filters:
    - event: system_event
      include: ["sys-1"]
`)
	// user: override adapter back to memory
	writeFile(t, user, "hooks.yaml", `
bus:
  adapter: memory
`)
	// project: leave adapter blank, append a stdout sink + filter
	writeFile(t, project, "hooks.yaml", `
bus:
  sinks:
    - type: stdout
  filters:
    - event: project_event
      exclude: ["proj-skip"]
`)

	paths := []string{
		filepath.Join(system, "hooks.yaml"),
		filepath.Join(user, "hooks.yaml"),
		filepath.Join(project, "hooks.yaml"),
	}
	cfg, err := loadFromPaths(paths)
	if err != nil {
		t.Fatal(err)
	}

	// User sets adapter=memory; project leaves blank → memory wins.
	if cfg.Bus.Adapter != "memory" {
		t.Errorf("adapter = %q, want memory (user override)", cfg.Bus.Adapter)
	}

	// Sinks append across layers.
	if len(cfg.Bus.Sinks) != 2 {
		t.Fatalf("sinks len = %d, want 2 (system+project)", len(cfg.Bus.Sinks))
	}
	if cfg.Bus.Sinks[0].Type != "jsonl" || cfg.Bus.Sinks[0].Path != "/var/log/nerv/system.jsonl" {
		t.Errorf("sinks[0] = %+v, want system jsonl", cfg.Bus.Sinks[0])
	}
	if cfg.Bus.Sinks[1].Type != "stdout" {
		t.Errorf("sinks[1].type = %q, want stdout", cfg.Bus.Sinks[1].Type)
	}

	// Filters append across layers.
	if len(cfg.Bus.Filters) != 2 {
		t.Fatalf("filters len = %d, want 2", len(cfg.Bus.Filters))
	}
	if cfg.Bus.Filters[0].Event != "system_event" {
		t.Errorf("filters[0].event = %q, want system_event", cfg.Bus.Filters[0].Event)
	}
	if cfg.Bus.Filters[1].Event != "project_event" {
		t.Errorf("filters[1].event = %q, want project_event", cfg.Bus.Filters[1].Event)
	}
}

func TestLoad_BusLayeredMergeNetworkFields(t *testing.T) {
	tmp := t.TempDir()
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")

	// user provides endpoint only; project provides auth only.
	// Field-level merge → both apply.
	writeFile(t, user, "hooks.yaml", `
bus:
  adapter: network
  network:
    endpoint: tcp://127.0.0.1:9000
`)
	writeFile(t, project, "hooks.yaml", `
bus:
  network:
    auth: secret-token
`)
	paths := []string{
		filepath.Join(user, "hooks.yaml"),
		filepath.Join(project, "hooks.yaml"),
	}
	cfg, err := loadFromPaths(paths)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bus.Adapter != "network" {
		t.Errorf("adapter = %q, want network", cfg.Bus.Adapter)
	}
	if cfg.Bus.Network.Endpoint != "tcp://127.0.0.1:9000" {
		t.Errorf("endpoint = %q", cfg.Bus.Network.Endpoint)
	}
	if cfg.Bus.Network.Auth != "secret-token" {
		t.Errorf("auth = %q", cfg.Bus.Network.Auth)
	}
}

func TestResolveOn_String(t *testing.T) {
	h := HookEntry{On: "PreToolUse"}
	got := h.ResolveOn()
	if len(got) != 1 || got[0] != "PreToolUse" {
		t.Errorf("ResolveOn(string) = %v, want [PreToolUse]", got)
	}
}

func TestResolveOn_Array(t *testing.T) {
	// yaml.v3 decodes []string as []any with string elements.
	h := HookEntry{On: []any{"PreToolUse", "PostToolUse"}}
	got := h.ResolveOn()
	if len(got) != 2 {
		t.Fatalf("ResolveOn(array) len = %d, want 2", len(got))
	}
	if got[0] != "PreToolUse" || got[1] != "PostToolUse" {
		t.Errorf("ResolveOn(array) = %v, want [PreToolUse PostToolUse]", got)
	}
}

// --- Builtin schema additions -------------------------------------------

func TestLoad_BuiltinRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
version: "1.0-draft"
builtin:
  open-questions:
    enabled: true
    drift_keywords:
      - "moving on"
      - "next:"
    state_ttl_hours: 168
`)
	cfg, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	oq, ok := cfg.Builtin["open-questions"]
	if !ok {
		t.Fatalf("Builtin[open-questions] missing; got keys %v", keysOfBuiltin(cfg.Builtin))
	}
	if !oq.EnabledOrDefault() {
		t.Errorf("Enabled = false, want true")
	}
	got, ok := oq.Extra["drift_keywords"].([]any)
	if !ok {
		t.Fatalf("drift_keywords missing or wrong type: %T", oq.Extra["drift_keywords"])
	}
	if len(got) != 2 || got[0] != "moving on" {
		t.Errorf("drift_keywords = %v, want [moving on, next:]", got)
	}
	if v, _ := oq.Extra["state_ttl_hours"].(int); v != 168 {
		t.Errorf("state_ttl_hours = %v, want 168", oq.Extra["state_ttl_hours"])
	}
}

// TestLoad_BuiltinFieldLevelMerge covers OQ1 / US-0006 AC#3:
// user layer sets `enabled: true`; project layer sets only
// drift_keywords; result: BOTH apply. Enabled is preserved from user
// layer because project does not redeclare the key.
func TestLoad_BuiltinFieldLevelMerge(t *testing.T) {
	tmp := t.TempDir()
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")

	writeFile(t, user, "hooks.yaml", `
version: "1.0-draft"
builtin:
  open-questions:
    enabled: true
    state_ttl_hours: 168
`)
	writeFile(t, project, "hooks.yaml", `
builtin:
  open-questions:
    drift_keywords: ["foo"]
`)

	cfg, err := loadFromPaths([]string{
		filepath.Join(user, "hooks.yaml"),
		filepath.Join(project, "hooks.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	oq := cfg.Builtin["open-questions"]
	if !oq.EnabledOrDefault() {
		t.Error("Enabled was clobbered by project layer; want preserved (true)")
	}
	if v, _ := oq.Extra["state_ttl_hours"].(int); v != 168 {
		t.Errorf("state_ttl_hours = %v, want 168 (preserved from user)", oq.Extra["state_ttl_hours"])
	}
	dk, _ := oq.Extra["drift_keywords"].([]any)
	if len(dk) != 1 || dk[0] != "foo" {
		t.Errorf("drift_keywords = %v, want [foo] (added by project)", dk)
	}
}

// TestLoad_BuiltinExplicitDisable confirms that an explicit
// `enabled: false` in a later layer DOES override an earlier `true`.
// Field-level merge means "wins when set", not "never overrides".
func TestLoad_BuiltinExplicitDisable(t *testing.T) {
	tmp := t.TempDir()
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")

	writeFile(t, user, "hooks.yaml", `
builtin:
  open-questions:
    enabled: true
`)
	writeFile(t, project, "hooks.yaml", `
builtin:
  open-questions:
    enabled: false
`)

	cfg, err := loadFromPaths([]string{
		filepath.Join(user, "hooks.yaml"),
		filepath.Join(project, "hooks.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	oq := cfg.Builtin["open-questions"]
	if oq.Enabled == nil {
		t.Fatal("Enabled = nil; want explicit false")
	}
	if oq.EnabledOrDefault() {
		t.Error("Enabled = true; want false (overridden by project)")
	}
}

// TestLoad_BuiltinExtraPerKeyMerge confirms Extra map merges per-key:
// user-only keys survive, project-only keys appear, collisions go to
// project.
func TestLoad_BuiltinExtraPerKeyMerge(t *testing.T) {
	tmp := t.TempDir()
	user := filepath.Join(tmp, "user")
	project := filepath.Join(tmp, "project")

	writeFile(t, user, "hooks.yaml", `
builtin:
  open-questions:
    state_ttl_hours: 24
    inbox_path: /user/inbox
`)
	writeFile(t, project, "hooks.yaml", `
builtin:
  open-questions:
    inbox_path: /project/inbox
    drift_keywords: ["x"]
`)
	cfg, err := loadFromPaths([]string{
		filepath.Join(user, "hooks.yaml"),
		filepath.Join(project, "hooks.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	oq := cfg.Builtin["open-questions"]
	if v, _ := oq.Extra["state_ttl_hours"].(int); v != 24 {
		t.Errorf("state_ttl_hours = %v, want 24 (user-only preserved)", oq.Extra["state_ttl_hours"])
	}
	if got, _ := oq.Extra["inbox_path"].(string); got != "/project/inbox" {
		t.Errorf("inbox_path = %q, want /project/inbox (project wins)", got)
	}
	if dk, _ := oq.Extra["drift_keywords"].([]any); len(dk) != 1 {
		t.Errorf("drift_keywords = %v, want [x] (project-only added)", dk)
	}
}

// TestLoad_BuiltinMultipleNames confirms separate built-in names live
// independently in the merged map.
func TestLoad_BuiltinMultipleNames(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
builtin:
  open-questions:
    enabled: true
  alpha-detector:
    enabled: false
    threshold: 5
`)
	cfg, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Builtin) != 2 {
		t.Fatalf("Builtin len = %d, want 2", len(cfg.Builtin))
	}
	if !cfg.Builtin["open-questions"].EnabledOrDefault() {
		t.Error("open-questions enabled = false, want true")
	}
	if cfg.Builtin["alpha-detector"].EnabledOrDefault() {
		t.Error("alpha-detector enabled = true, want false")
	}
	if v, _ := cfg.Builtin["alpha-detector"].Extra["threshold"].(int); v != 5 {
		t.Errorf("alpha-detector threshold = %v, want 5", cfg.Builtin["alpha-detector"].Extra["threshold"])
	}
}

// TestEnabledOrDefault covers the helper directly so callers can rely
// on its three states (nil, &false, &true).
func TestEnabledOrDefault(t *testing.T) {
	cases := []struct {
		name string
		in   *bool
		want bool
	}{
		{"nil", nil, false},
		{"false", boolPtr(false), false},
		{"true", boolPtr(true), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := BuiltinUserCfg{Enabled: c.in}
			if got := b.EnabledOrDefault(); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

// TestHookEntry_SourceNotSerialized confirms the runtime-only Source
// field is preserved on a HookEntry but never written to YAML.
func TestHookEntry_SourceNotSerialized(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
hooks:
  - name: lint
    on: PreToolUse
    run: ./lint.sh
`)
	cfg, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Hooks[0].Source != "" {
		t.Errorf("Source = %q on YAML-loaded hook, want empty (zero-value)", cfg.Hooks[0].Source)
	}

	// Setting Source post-load works (runtime injection).
	cfg.Hooks[0].Source = "builtin"
	if cfg.Hooks[0].Source != "builtin" {
		t.Errorf("post-set Source = %q, want builtin", cfg.Hooks[0].Source)
	}
}

func boolPtr(b bool) *bool { return &b }

func keysOfBuiltin(m map[string]BuiltinUserCfg) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestLoadDisabledProject_WalksUpFromSubdir locks in the fix for the
// daemon vs CLI sidecar-lookup asymmetry. Before the fix
// LoadDisabledProject was cwd-relative (filepath.Join(".nerv",
// "disabled.yaml")), so a daemon started from <project>/sub/dir/
// silently read nothing while `nerv disable` happily wrote to
// <project>/.nerv/disabled.yaml.
//
// Regression: LoadDisabledProject now walks up to find .nerv/, so
// the read path matches the write path regardless of cwd.
func TestLoadDisabledProject_WalksUpFromSubdir(t *testing.T) {
	// Set up <tmp>/proj/.nerv/disabled.yaml + <tmp>/proj/sub/dir/.
	proj := filepath.Join(t.TempDir(), "proj")
	nervDir := filepath.Join(proj, ".nerv")
	if err := os.MkdirAll(nervDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(proj, "sub", "dir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	sidecar := `version: "1.0-draft"
events:
  - PreToolUse
bindings:
  - {hook: foo, event: PostToolUse}
`
	if err := os.WriteFile(filepath.Join(nervDir, "disabled.yaml"),
		[]byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}

	// chdir into the deep subdirectory — mimics a daemon launched from
	// something other than the project root.
	t.Chdir(sub)

	d := LoadDisabledProject()
	if d == nil {
		t.Fatal("LoadDisabledProject returned nil")
	}
	if !d.HasEvent("PreToolUse") {
		t.Errorf("HasEvent(PreToolUse) = false, want true (sidecar not loaded from project root)")
	}
	if !d.HasBinding("foo", "PostToolUse") {
		t.Errorf("HasBinding(foo,PostToolUse) = false, want true")
	}
}

// TestLoadDisabledProject_NoProject confirms a daemon running outside
// any nerv project still gets a usable zero-value sidecar (fail-open).
func TestLoadDisabledProject_NoProject(t *testing.T) {
	// chdir into a temp directory with no .nerv/ anywhere on the path
	// upward. (TempDir is somewhere under /tmp or /var/folders; walk-up
	// hits the FS root without finding a marker.)
	t.Chdir(t.TempDir())

	d := LoadDisabledProject()
	if d == nil {
		t.Fatal("LoadDisabledProject returned nil")
	}
	if d.Version != DisabledSidecarVersion {
		t.Errorf("Version = %q, want %q", d.Version, DisabledSidecarVersion)
	}
	if len(d.Events) != 0 || len(d.Bindings) != 0 {
		t.Errorf("expected zero-value sidecar, got events=%v bindings=%v",
			d.Events, d.Bindings)
	}
}

// TestFindProjectRootFrom_WalksUp covers the hoisted helper directly:
// given a deep subdirectory, the walk-up resolves to the directory
// containing .nerv/.
func TestFindProjectRootFrom_WalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".nerv"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := FindProjectRootFrom(deep)
	if err != nil {
		t.Fatalf("FindProjectRootFrom: %v", err)
	}
	gotEval, _ := filepath.EvalSymlinks(got)
	wantEval, _ := filepath.EvalSymlinks(root)
	if gotEval != wantEval {
		t.Errorf("project root = %s, want %s", gotEval, wantEval)
	}
}

func TestFindProjectRootFrom_ErrNotProject(t *testing.T) {
	if _, err := FindProjectRootFrom(t.TempDir()); err != ErrNotProject {
		t.Fatalf("expected ErrNotProject, got %v", err)
	}
}

func TestLoad_IngestRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
version: "1.0-draft"
hooks: []
ingest:
  enabled: true
  scan_roots:
    - /var/lib/agents/stem/sessions
    - /var/lib/agents/other/sessions
  scan_interval_seconds: 12
`)
	cfg, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Ingest.EnabledOrDefault() {
		t.Error("ingest.enabled = false, want true")
	}
	if len(cfg.Ingest.ScanRoots) != 2 || cfg.Ingest.ScanRoots[0] != "/var/lib/agents/stem/sessions" {
		t.Errorf("scan_roots = %v", cfg.Ingest.ScanRoots)
	}
	if cfg.Ingest.ScanIntervalSeconds != 12 {
		t.Errorf("scan_interval_seconds = %d, want 12", cfg.Ingest.ScanIntervalSeconds)
	}
}

func TestLoad_IngestDefaults(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
version: "1.0-draft"
hooks: []
`)
	cfg, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	// Absent section: consumption is on by default (daemon-side gating
	// decides whether anything actually attaches).
	if !cfg.Ingest.EnabledOrDefault() {
		t.Error("ingest defaults to disabled; want enabled")
	}
	if len(cfg.Ingest.ScanRoots) != 0 {
		t.Errorf("default scan_roots = %v, want none", cfg.Ingest.ScanRoots)
	}

	// Explicit opt-out sticks.
	writeFile(t, tmp, "hooks.yaml", `
ingest:
  enabled: false
`)
	cfg, err = loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Ingest.EnabledOrDefault() {
		t.Error("explicit ingest.enabled=false ignored")
	}
}

func TestLoad_IngestValidation(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "hooks.yaml", `
ingest:
  scan_roots:
    - relative/not/allowed
`)
	if _, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")}); err == nil {
		t.Fatal("expected ErrConfig for relative scan root, got nil")
	}

	writeFile(t, tmp, "hooks.yaml", `
ingest:
  scan_interval_seconds: -3
`)
	if _, err := loadFromPaths([]string{filepath.Join(tmp, "hooks.yaml")}); err == nil {
		t.Fatal("expected ErrConfig for negative scan interval, got nil")
	}
}

func TestLoad_IngestLayerMerge(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, tmp, "user.yaml", `
ingest:
  enabled: false
  scan_roots: [/from/user]
`)
	writeFile(t, tmp, "project.yaml", `
ingest:
  scan_roots: [/from/project]
  scan_interval_seconds: 30
`)
	cfg, err := loadFromPaths([]string{
		filepath.Join(tmp, "user.yaml"),
		filepath.Join(tmp, "project.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Scalar merge: project layer did not set enabled → user's false
	// survives; interval comes from project; roots append.
	if cfg.Ingest.EnabledOrDefault() {
		t.Error("enabled=false from user layer clobbered by project omission")
	}
	if cfg.Ingest.ScanIntervalSeconds != 30 {
		t.Errorf("scan_interval_seconds = %d, want 30", cfg.Ingest.ScanIntervalSeconds)
	}
	if len(cfg.Ingest.ScanRoots) != 2 {
		t.Errorf("scan_roots = %v, want both layers", cfg.Ingest.ScanRoots)
	}
}
