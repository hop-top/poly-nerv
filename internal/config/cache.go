package config

import (
	"encoding/gob"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hop.top/kit/go/core/xdg"
)

func init() {
	// HookEntry.On is typed as `any` and may hold string or []any.
	// BuiltinUserCfg.Extra is map[string]any; nested YAML values may
	// also be map[string]any, int, or bool. gob requires concrete
	// types behind interfaces to be registered.
	gob.Register("")
	gob.Register([]any{})
	gob.Register(map[string]any{})
	gob.Register(0)
	gob.Register(false)
}

// CachedConfig is the pre-compiled, indexed hook config.
//
// ActiveProfile is set at compile time from the live aps lookup.
// It is NOT serialised from user yaml — yaml-driven values
// would be stale relative to the running daemon's environment.
// Empty string means no active profile (aps not in use, or lookup
// failed); built-ins that depend on it MUST treat empty as a fail-open
// signal (skip token expansion, use a sane default path).
type CachedConfig struct {
	Config          *HooksConfig
	ByEvent         map[string][]HookEntry // indexed by event name for O(1) lookup
	Version         string                 // nerv version stamp for cache invalidation
	SourceMod       time.Time              // latest mtime of source hooks.yaml files
	BuiltinVersions map[string]int         // builtin name -> SchemaVersion at compile time
	ActiveProfile   string                 // aps profile id at daemon startup; "" if unknown
	// Disabled holds the parsed .nerv/disabled.yaml sidecar; dispatch
	// consults it to skip events + bindings without re-reading the
	// file per envelope. Populated by Compile (current process state);
	// re-populated on daemon reload.
	Disabled *Disabled
}

// Compile indexes a HooksConfig for fast dispatch lookup.
//
// BuiltinVersions is initialised to an empty (non-nil) map so callers
// can populate it without nil-check ceremony. builtins.Inject
// populates it; today this stays empty.
//
// ActiveProfile is populated via LookupActiveProfile(); a daemon
// running outside an aps context (no APS_PROFILE_ID env var) leaves
// it empty. Callers that need to override (tests, cmd/nerv tooling
// running ad-hoc) set the field directly after Compile returns.
func Compile(cfg *HooksConfig) *CachedConfig {
	byEvent := make(map[string][]HookEntry)
	for _, h := range cfg.Hooks {
		for _, ev := range h.ResolveOn() {
			byEvent[ev] = append(byEvent[ev], h)
		}
	}
	return &CachedConfig{
		Config:          cfg,
		ByEvent:         byEvent,
		BuiltinVersions: make(map[string]int),
		ActiveProfile:   LookupActiveProfile(),
		Disabled:        LoadDisabledProject(),
	}
}

// LookupActiveProfile resolves the running daemon's aps profile id.
//
// aps injects APS_PROFILE_ID into a process's environment when it
// runs an action under a profile (see aps spec 004-profile-env-prefix).
// Until aps exposes a Go API for "current profile" we read the env
// var directly — this is the canonical channel and matches how
// adapters/messengers already discover profile identity.
//
// Returns "" when unset; per nerv fail-open contract the daemon must
// keep starting.
func LookupActiveProfile() string {
	return strings.TrimSpace(os.Getenv("APS_PROFILE_ID"))
}

// ResolveTokens expands path/template tokens against cfg's runtime
// fields. Currently supported:
//
//	<agent_id>  → cfg.ActiveProfile (empty when not running under aps)
//
// Unknown tokens are left intact so the caller can decide how to
// surface the omission. Consumers (BuiltinRuntimeCfg.InboxDir
// resolution) call this on user-supplied path templates such as
// "$XDG_STATE_HOME/nerv/inbox/<agent_id>/".
//
// Designed to grow: add new tokens by appending to the replacement
// table; never branch per-mode at the call site.
func ResolveTokens(template string, cfg *CachedConfig) string {
	if cfg == nil {
		return template
	}
	replacements := []struct {
		token, value string
	}{
		{"<agent_id>", cfg.ActiveProfile},
	}
	out := template
	for _, r := range replacements {
		if r.value == "" {
			continue
		}
		out = strings.ReplaceAll(out, r.token, r.value)
	}
	return out
}

// DefaultCachePath returns $XDG_CACHE_HOME/nerv/hooks.cache
// via kit/xdg (honors XDG_CACHE_HOME with OS-native fallbacks).
func DefaultCachePath() (string, error) {
	dir, err := xdg.CacheDir("nerv")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "hooks.cache"), nil
}

// SaveCache serializes CachedConfig to gob at cachePath.
// Uses atomic temp+rename for safety.
func SaveCache(cc *CachedConfig, cachePath string) error {
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	f, err := os.CreateTemp(dir, "hooks.cache.*.tmp")
	if err != nil {
		return err
	}
	tmpName := f.Name()

	if err := gob.NewEncoder(f).Encode(cc); err != nil {
		_ = f.Close()          // best-effort cleanup; the encode error is returned
		_ = os.Remove(tmpName) // best-effort cleanup
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpName) // best-effort cleanup; the close error is returned
		return err
	}

	return os.Rename(tmpName, cachePath)
}

// LoadCache reads cached config from cachePath.
// Returns nil if: file missing, corrupt, version mismatch, source newer.
func LoadCache(cachePath string, currentVersion string, sourceMod time.Time) *CachedConfig {
	f, err := os.Open(cachePath)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }() // read-only

	var cc CachedConfig
	if err := gob.NewDecoder(f).Decode(&cc); err != nil {
		return nil
	}

	if cc.Version != currentVersion {
		return nil
	}

	if sourceMod.After(cc.SourceMod) {
		return nil
	}

	return &cc
}
