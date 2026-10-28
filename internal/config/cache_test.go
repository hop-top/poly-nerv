package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompile_IndexesByEvent(t *testing.T) {
	cfg := &HooksConfig{
		Hooks: []HookEntry{
			{Name: "lint", On: "PreToolUse", Run: "./lint.sh"},
			{Name: "log", On: "PostToolUse", Run: "./log.sh"},
			{Name: "guard", On: "PreToolUse", Run: "./guard.sh"},
		},
	}

	cc := Compile(cfg)

	if len(cc.ByEvent["PreToolUse"]) != 2 {
		t.Fatalf("PreToolUse hooks = %d, want 2", len(cc.ByEvent["PreToolUse"]))
	}
	if cc.ByEvent["PreToolUse"][0].Name != "lint" {
		t.Errorf("first PreToolUse hook = %q, want lint", cc.ByEvent["PreToolUse"][0].Name)
	}
	if cc.ByEvent["PreToolUse"][1].Name != "guard" {
		t.Errorf("second PreToolUse hook = %q, want guard", cc.ByEvent["PreToolUse"][1].Name)
	}
	if len(cc.ByEvent["PostToolUse"]) != 1 {
		t.Fatalf("PostToolUse hooks = %d, want 1", len(cc.ByEvent["PostToolUse"]))
	}
	if cc.ByEvent["PostToolUse"][0].Name != "log" {
		t.Errorf("PostToolUse hook = %q, want log", cc.ByEvent["PostToolUse"][0].Name)
	}
	if cc.Config != cfg {
		t.Error("Config pointer mismatch")
	}
}

func TestCompile_MultiEventHook(t *testing.T) {
	cfg := &HooksConfig{
		Hooks: []HookEntry{
			{Name: "audit", On: []any{"PreToolUse", "PostToolUse"}, Run: "./audit.sh"},
		},
	}

	cc := Compile(cfg)

	if len(cc.ByEvent["PreToolUse"]) != 1 {
		t.Fatalf("PreToolUse hooks = %d, want 1", len(cc.ByEvent["PreToolUse"]))
	}
	if cc.ByEvent["PreToolUse"][0].Name != "audit" {
		t.Errorf("PreToolUse hook = %q, want audit", cc.ByEvent["PreToolUse"][0].Name)
	}
	if len(cc.ByEvent["PostToolUse"]) != 1 {
		t.Fatalf("PostToolUse hooks = %d, want 1", len(cc.ByEvent["PostToolUse"]))
	}
	if cc.ByEvent["PostToolUse"][0].Name != "audit" {
		t.Errorf("PostToolUse hook = %q, want audit", cc.ByEvent["PostToolUse"][0].Name)
	}
}

func TestSaveLoadCache_Roundtrip(t *testing.T) {
	cfg := &HooksConfig{
		Version: "1.0-draft",
		Defaults: HookDefaults{
			Timeout: 5,
			OnError: "abort",
		},
		Hooks: []HookEntry{
			{Name: "lint", On: "PreToolUse", Run: "./lint.sh"},
			{Name: "audit", On: "PostToolUse", Run: "./audit.sh"},
		},
	}

	cc := Compile(cfg)
	cc.Version = "v0.1.0"
	cc.SourceMod = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	cachePath := filepath.Join(t.TempDir(), "nerv", "hooks.cache")

	if err := SaveCache(cc, cachePath); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	loaded := LoadCache(cachePath, "v0.1.0", cc.SourceMod)
	if loaded == nil {
		t.Fatal("LoadCache returned nil")
	}

	if loaded.Version != cc.Version {
		t.Errorf("version = %q, want %q", loaded.Version, cc.Version)
	}
	if !loaded.SourceMod.Equal(cc.SourceMod) {
		t.Errorf("sourceMod = %v, want %v", loaded.SourceMod, cc.SourceMod)
	}
	if loaded.Config.Version != cfg.Version {
		t.Errorf("config version = %q, want %q", loaded.Config.Version, cfg.Version)
	}
	if len(loaded.Config.Hooks) != 2 {
		t.Fatalf("hooks len = %d, want 2", len(loaded.Config.Hooks))
	}
	if len(loaded.ByEvent["PreToolUse"]) != 1 {
		t.Errorf("PreToolUse hooks = %d, want 1", len(loaded.ByEvent["PreToolUse"]))
	}
	if len(loaded.ByEvent["PostToolUse"]) != 1 {
		t.Errorf("PostToolUse hooks = %d, want 1", len(loaded.ByEvent["PostToolUse"]))
	}
}

func TestLoadCache_VersionMismatch(t *testing.T) {
	cfg := &HooksConfig{
		Hooks: []HookEntry{
			{Name: "lint", On: "PreToolUse", Run: "./lint.sh"},
		},
	}
	cc := Compile(cfg)
	cc.Version = "v0.1.0"
	cc.SourceMod = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	cachePath := filepath.Join(t.TempDir(), "hooks.cache")
	if err := SaveCache(cc, cachePath); err != nil {
		t.Fatal(err)
	}

	loaded := LoadCache(cachePath, "v0.2.0", cc.SourceMod)
	if loaded != nil {
		t.Error("expected nil on version mismatch")
	}
}

func TestLoadCache_SourceNewer(t *testing.T) {
	cfg := &HooksConfig{
		Hooks: []HookEntry{
			{Name: "lint", On: "PreToolUse", Run: "./lint.sh"},
		},
	}
	cc := Compile(cfg)
	cc.Version = "v0.1.0"
	cc.SourceMod = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	cachePath := filepath.Join(t.TempDir(), "hooks.cache")
	if err := SaveCache(cc, cachePath); err != nil {
		t.Fatal(err)
	}

	newerSource := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	loaded := LoadCache(cachePath, "v0.1.0", newerSource)
	if loaded != nil {
		t.Error("expected nil when source is newer than cache")
	}
}

func TestLoadCache_Corrupt(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "hooks.cache")
	if err := os.WriteFile(cachePath, []byte("not gob data!!!"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := LoadCache(cachePath, "v0.1.0", time.Now())
	if loaded != nil {
		t.Error("expected nil on corrupt cache file")
	}
}

func TestLoadCache_Missing(t *testing.T) {
	loaded := LoadCache("/nonexistent/hooks.cache", "v0.1.0", time.Now())
	if loaded != nil {
		t.Error("expected nil on missing cache file")
	}
}

// --- BuiltinVersions ------------------------------------------------

// TestCompile_BuiltinVersionsInitialised confirms Compile produces a
// non-nil BuiltinVersions map. Callers populate it when
// builtins.Inject runs; today it stays empty but must be allocated.
func TestCompile_BuiltinVersionsInitialised(t *testing.T) {
	cc := Compile(&HooksConfig{})
	if cc.BuiltinVersions == nil {
		t.Fatal("BuiltinVersions = nil, want non-nil empty map")
	}
	if len(cc.BuiltinVersions) != 0 {
		t.Errorf("BuiltinVersions len = %d, want 0", len(cc.BuiltinVersions))
	}
}

// TestSaveLoadCache_BuiltinVersionsRoundtrip verifies the new field
// survives gob round-trip. Future cache misses must not silently drop
// version state.
func TestSaveLoadCache_BuiltinVersionsRoundtrip(t *testing.T) {
	cfg := &HooksConfig{Version: "1.0-draft"}
	cc := Compile(cfg)
	cc.Version = "v0.1.0"
	cc.SourceMod = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cc.BuiltinVersions["open-questions"] = 1
	cc.BuiltinVersions["alpha-detector"] = 3

	cachePath := filepath.Join(t.TempDir(), "hooks.cache")
	if err := SaveCache(cc, cachePath); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	loaded := LoadCache(cachePath, "v0.1.0", cc.SourceMod)
	if loaded == nil {
		t.Fatal("LoadCache returned nil")
	}
	if got := loaded.BuiltinVersions["open-questions"]; got != 1 {
		t.Errorf("open-questions version = %d, want 1", got)
	}
	if got := loaded.BuiltinVersions["alpha-detector"]; got != 3 {
		t.Errorf("alpha-detector version = %d, want 3", got)
	}
	if len(loaded.BuiltinVersions) != 2 {
		t.Errorf("BuiltinVersions len = %d, want 2", len(loaded.BuiltinVersions))
	}
}

// --- ActiveProfile + ResolveTokens ----------------------------------

// TestLookupActiveProfile_EnvVar verifies the env-driven lookup
// honours APS_PROFILE_ID.
func TestLookupActiveProfile_EnvVar(t *testing.T) {
	tests := []struct {
		name string
		envs map[string]string
		want string
	}{
		{
			name: "APS_PROFILE_ID set",
			envs: map[string]string{"APS_PROFILE_ID": "test-profile"},
			want: "test-profile",
		},
		{
			name: "whitespace trimmed",
			envs: map[string]string{"APS_PROFILE_ID": "  test-profile  "},
			want: "test-profile",
		},
		{
			name: "unset",
			envs: map[string]string{},
			want: "",
		},
		{
			name: "empty string treated as unset",
			envs: map[string]string{"APS_PROFILE_ID": ""},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("APS_PROFILE_ID", "")
			for k, v := range tc.envs {
				t.Setenv(k, v)
			}
			if got := LookupActiveProfile(); got != tc.want {
				t.Errorf("LookupActiveProfile() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCompile_ActiveProfileFromEnv verifies Compile threads the env
// lookup into CachedConfig — so daemon startup gets a populated field
// without explicit wiring at the call site.
func TestCompile_ActiveProfileFromEnv(t *testing.T) {
	t.Setenv("APS_PROFILE_ID", "test-profile")

	cc := Compile(&HooksConfig{})
	if cc.ActiveProfile != "test-profile" {
		t.Errorf("ActiveProfile = %q, want %q", cc.ActiveProfile, "test-profile")
	}
}

// TestCompile_ActiveProfileEmptyWhenUnset asserts fail-open semantics:
// no aps env → empty string, daemon still gets a usable config.
func TestCompile_ActiveProfileEmptyWhenUnset(t *testing.T) {
	t.Setenv("APS_PROFILE_ID", "")

	cc := Compile(&HooksConfig{})
	if cc.ActiveProfile != "" {
		t.Errorf("ActiveProfile = %q, want empty", cc.ActiveProfile)
	}
}

// TestSaveLoadCache_ActiveProfileRoundtrip locks in gob round-trip
// for the ActiveProfile field. Set at compile time per invocation;
// LoadCache callers refresh it from a live lookup, but the field
// must still survive serialisation so cached values are readable.
func TestSaveLoadCache_ActiveProfileRoundtrip(t *testing.T) {
	cc := Compile(&HooksConfig{Version: "1.0-draft"})
	cc.Version = "v0.1.0"
	cc.SourceMod = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cc.ActiveProfile = "test-profile"

	cachePath := filepath.Join(t.TempDir(), "hooks.cache")
	if err := SaveCache(cc, cachePath); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	loaded := LoadCache(cachePath, "v0.1.0", cc.SourceMod)
	if loaded == nil {
		t.Fatal("LoadCache returned nil")
	}
	if loaded.ActiveProfile != "test-profile" {
		t.Errorf("ActiveProfile = %q, want %q", loaded.ActiveProfile, "test-profile")
	}
}

// TestResolveTokens covers <agent_id> expansion, missing-profile
// fail-open (token left intact), and unknown-token passthrough.
func TestResolveTokens(t *testing.T) {
	tests := []struct {
		name     string
		template string
		profile  string
		want     string
	}{
		{
			name:     "agent_id expanded",
			template: "$XDG_STATE_HOME/nerv/inbox/<agent_id>/",
			profile:  "test-profile",
			want:     "$XDG_STATE_HOME/nerv/inbox/test-profile/",
		},
		{
			name:     "multiple occurrences",
			template: "<agent_id>/work/<agent_id>",
			profile:  "test-profile",
			want:     "test-profile/work/test-profile",
		},
		{
			name:     "no token, no rewrite",
			template: "/tmp/inbox/",
			profile:  "test-profile",
			want:     "/tmp/inbox/",
		},
		{
			name:     "empty profile preserves token",
			template: "/inbox/<agent_id>/",
			profile:  "",
			want:     "/inbox/<agent_id>/",
		},
		{
			name:     "unknown token left alone",
			template: "/inbox/<agent_id>/<workspace>/",
			profile:  "test-profile",
			want:     "/inbox/test-profile/<workspace>/",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cc := &CachedConfig{ActiveProfile: tc.profile}
			if got := ResolveTokens(tc.template, cc); got != tc.want {
				t.Errorf("ResolveTokens(%q) = %q, want %q", tc.template, got, tc.want)
			}
		})
	}
}

// TestResolveTokens_NilCfg guards against nil dereference when callers
// haven't built a CachedConfig yet (early-startup paths).
func TestResolveTokens_NilCfg(t *testing.T) {
	got := ResolveTokens("/inbox/<agent_id>/", nil)
	if got != "/inbox/<agent_id>/" {
		t.Errorf("ResolveTokens(nil) = %q, want template unchanged", got)
	}
}

// --- Disabled sidecar (review #3) -----------------------------------

// TestSaveLoadCache_DisabledRoundtrip locks in gob round-trip for the
// Disabled sidecar pointer embedded in CachedConfig. Daemon startup
// reads .nerv/disabled.yaml once at compile time; cache entries must
// preserve that snapshot so a SIGHUP-less reload still skips the
// disabled events/bindings. A schema change to Disabled that breaks
// gob round-trip would silently allow dispatch to fire on entries the
// user has explicitly disabled.
func TestSaveLoadCache_DisabledRoundtrip(t *testing.T) {
	cfg := &HooksConfig{Version: "1.0-draft"}
	cc := Compile(cfg)
	cc.Version = "v0.1.0"
	cc.SourceMod = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cc.Disabled = &Disabled{
		Version: DisabledSidecarVersion,
		Events:  []string{"PreToolUse", "Stop"},
		Bindings: []DisabledBinding{
			{Hook: "lint", Event: "PreToolUse"},
			{Hook: "audit", Event: "PostToolUse"},
		},
	}

	cachePath := filepath.Join(t.TempDir(), "hooks.cache")
	if err := SaveCache(cc, cachePath); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	loaded := LoadCache(cachePath, "v0.1.0", cc.SourceMod)
	if loaded == nil {
		t.Fatal("LoadCache returned nil")
	}
	if loaded.Disabled == nil {
		t.Fatal("Disabled lost in round-trip")
	}
	if loaded.Disabled.Version != DisabledSidecarVersion {
		t.Errorf("Disabled.Version = %q, want %q",
			loaded.Disabled.Version, DisabledSidecarVersion)
	}
	if len(loaded.Disabled.Events) != 2 {
		t.Fatalf("Disabled.Events len = %d, want 2",
			len(loaded.Disabled.Events))
	}
	if !loaded.Disabled.HasEvent("PreToolUse") || !loaded.Disabled.HasEvent("Stop") {
		t.Errorf("Disabled.Events = %v, want [PreToolUse Stop]",
			loaded.Disabled.Events)
	}
	if len(loaded.Disabled.Bindings) != 2 {
		t.Fatalf("Disabled.Bindings len = %d, want 2",
			len(loaded.Disabled.Bindings))
	}
	if !loaded.Disabled.HasBinding("lint", "PreToolUse") {
		t.Errorf("missing binding lint/PreToolUse: %v",
			loaded.Disabled.Bindings)
	}
	if !loaded.Disabled.HasBinding("audit", "PostToolUse") {
		t.Errorf("missing binding audit/PostToolUse: %v",
			loaded.Disabled.Bindings)
	}
}

// TestSaveLoadCache_DisabledNilRoundtrip locks in nil-safety: when the
// sidecar is absent cc.Disabled is nil, and LoadCache must surface that
// nil verbatim through the gob round-trip — splitDisabled handles a nil
// receiver, but only if the round-trip preserves it.
func TestSaveLoadCache_DisabledNilRoundtrip(t *testing.T) {
	cc := Compile(&HooksConfig{Version: "1.0-draft"})
	cc.Version = "v0.1.0"
	cc.SourceMod = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	cc.Disabled = nil // explicit nil — sidecar absent

	cachePath := filepath.Join(t.TempDir(), "hooks.cache")
	if err := SaveCache(cc, cachePath); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	loaded := LoadCache(cachePath, "v0.1.0", cc.SourceMod)
	if loaded == nil {
		t.Fatal("LoadCache returned nil")
	}
	if loaded.Disabled != nil {
		t.Errorf("Disabled = %+v, want nil after round-trip", loaded.Disabled)
	}
	// Nil receiver helpers must remain safe.
	if loaded.Disabled.HasEvent("PreToolUse") {
		t.Error("nil Disabled.HasEvent returned true")
	}
	if loaded.Disabled.HasBinding("h", "e") {
		t.Error("nil Disabled.HasBinding returned true")
	}
}

// TestSaveLoadCache_BuiltinConfigRoundtrip confirms the new HooksConfig
// Builtin map survives cache round-trip with both Enabled and Extra
// fields intact, including nested map[string]any and []any values
// (proves required gob.Register calls in cache.go init).
func TestSaveLoadCache_BuiltinConfigRoundtrip(t *testing.T) {
	enabled := true
	cfg := &HooksConfig{
		Version: "1.0-draft",
		Builtin: map[string]BuiltinUserCfg{
			"open-questions": {
				Enabled: &enabled,
				Extra: map[string]any{
					"state_ttl_hours": 168,
					"inbox_path":      "/tmp/inbox",
					"drift_keywords":  []any{"moving on", "next:"},
					"llm": map[string]any{
						"provider":      "nerv-primary",
						"budget_tokens": 2000,
					},
				},
			},
		},
	}
	cc := Compile(cfg)
	cc.Version = "v0.1.0"
	cc.SourceMod = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	cachePath := filepath.Join(t.TempDir(), "hooks.cache")
	if err := SaveCache(cc, cachePath); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	loaded := LoadCache(cachePath, "v0.1.0", cc.SourceMod)
	if loaded == nil {
		t.Fatal("LoadCache returned nil")
	}
	oq, ok := loaded.Config.Builtin["open-questions"]
	if !ok {
		t.Fatal("open-questions missing after round-trip")
	}
	if !oq.EnabledOrDefault() {
		t.Error("Enabled lost in round-trip")
	}
	if v, _ := oq.Extra["state_ttl_hours"].(int); v != 168 {
		t.Errorf("state_ttl_hours = %v, want 168", oq.Extra["state_ttl_hours"])
	}
	if got, _ := oq.Extra["inbox_path"].(string); got != "/tmp/inbox" {
		t.Errorf("inbox_path = %q, want /tmp/inbox", got)
	}
	dk, _ := oq.Extra["drift_keywords"].([]any)
	if len(dk) != 2 || dk[0] != "moving on" {
		t.Errorf("drift_keywords = %v, want [moving on, next:]", dk)
	}
	llm, _ := oq.Extra["llm"].(map[string]any)
	if llm == nil {
		t.Fatalf("llm nested map lost: %T", oq.Extra["llm"])
	}
	if got, _ := llm["provider"].(string); got != "nerv-primary" {
		t.Errorf("llm.provider = %q, want nerv-primary", got)
	}
}
