package guardrails

import (
	"fmt"
	"os"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/hookpkg/builtins"
	"hop.top/nerv/internal/merger"
)

// Def is the registry entry for the guardrails built-in, wired
// into builtins.All via init().
var Def = builtins.BuiltinDef{
	Name:          "guardrails",
	SchemaVersion: 1,
	Events:        []string{"PreToolUse"},
	Func:          Handle,
}

func init() {
	builtins.All = append(builtins.All, Def)
}

// Config is the strongly-typed view of UserCfg.Extra under
// hooks.builtin.guardrails.* . v1 only honours `enabled` (default
// true) and the per-rule mode override map. Backup tuning + GC
// thresholds live in this struct but are not yet read at runtime
// (pending GC scheduler).
type Config struct {
	Enabled bool              `yaml:"enabled"`
	Rules   map[string]string `yaml:"rules"`
	Backup  BackupConfig      `yaml:"backup"`
}

// BackupConfig is the persistent retention knob block.
type BackupConfig struct {
	TTLHours      int   `yaml:"ttl_hours"`
	MaxPerProject int   `yaml:"max_per_project"`
	MaxTotalBytes int64 `yaml:"max_total_bytes"`
}

// Handle is the per-event entry point dispatch calls. Returns Allow
// + nil error on every "no-rule-fired" path so the daemon never
// fails-closed on guardrail evaluation.
//
// Decision flow:
//
//  1. Decode user config; if disabled, allow.
//  2. Load packs (cached after first call — see packCache).
//  3. Run Matcher.Evaluate.
//  4. If destructive AND action != Block, capture backup; on
//     backup failure upgrade to Block.
//  5. Map Outcome to merger.Decision.
//
// HITL / manager / peer-N modes are not yet active — they fall
// through to Block in v1 (matcher.outcomeFor handles this).
func Handle(env *envelope.Envelope, cfg builtins.BuiltinRuntimeCfg) (merger.Decision, error) {
	gcfg := decodeConfig(cfg.UserCfg)
	if !gcfg.Enabled {
		return merger.Decision{Action: merger.Allow}, nil
	}

	packs, err := loadCachedPacks()
	if err != nil {
		// Fail-open: if rule packs cannot load (e.g. bad YAML), let
		// the command through. The daemon logs the error elsewhere.
		return merger.Decision{Action: merger.Allow}, nil
	}

	out := NewMatcher().Evaluate(env, packs, gcfg.Rules)

	// Backup before allowing destructive mutations through.
	if out.Destructive && out.Action != merger.Block {
		eng := NewBackupEngine(cfg.StateDir)
		_, berr := eng.Capture(BackupRequest{
			RuleID:   out.RuleID,
			Strategy: out.BackupStrategy,
			CWD:      currentCWD(env),
			Command:  commandFromOutcomeContext(env),
		})
		if berr != nil {
			// Backup failed → upgrade to Block per ADR.
			return merger.Decision{
				Action:  merger.Block,
				Message: fmt.Sprintf("guardrails(%s): backup failed: %v", out.RuleID, berr),
			}, nil
		}
	}

	return outcomeToDecision(out), nil
}

// outcomeToDecision is the bridge between guardrails.Outcome and
// merger.Decision. Allow → Allow; Block → Block; Rewrite → Rewrite
// with the rewritten command in Rewrite payload + rule + reason in
// Metadata so the merger / dispatch can surface them.
func outcomeToDecision(out Outcome) merger.Decision {
	d := merger.Decision{Action: out.Action, Message: out.Reason}
	if out.RuleID != "" {
		d.Metadata = map[string]any{"guardrails_rule": out.RuleID}
	}
	if out.Action == merger.Rewrite {
		d.Rewrite = map[string]any{"command": out.RewrittenCommand}
	}
	return d
}

// decodeConfig converts the raw UserCfg map (post-layer-merge) into
// a typed Config. Defaults: enabled=true, no overrides.
//
// We hand-decode rather than yaml.Unmarshal a re-marshal because the
// inline map is already typed any/map[string]any from the YAML
// loader.
func decodeConfig(raw map[string]any) Config {
	cfg := Config{Enabled: true} // default-on
	if raw == nil {
		return cfg
	}
	if v, ok := raw["enabled"].(bool); ok {
		cfg.Enabled = v
	}
	if v, ok := raw["rules"].(map[string]any); ok {
		cfg.Rules = make(map[string]string, len(v))
		for k, mv := range v {
			if s, ok := mv.(string); ok {
				cfg.Rules[k] = s
			} else if m, ok := mv.(map[string]any); ok {
				if s, ok := m["mode"].(string); ok {
					cfg.Rules[k] = s
				}
			}
		}
	}
	if v, ok := raw["backup"].(map[string]any); ok {
		if h, ok := v["ttl_hours"].(int); ok {
			cfg.Backup.TTLHours = h
		}
		if m, ok := v["max_per_project"].(int); ok {
			cfg.Backup.MaxPerProject = m
		}
		if b, ok := v["max_total_bytes"].(int64); ok {
			cfg.Backup.MaxTotalBytes = b
		}
	}
	return cfg
}

// currentCWD extracts the cwd from the envelope's session info,
// falling back to os.Getwd(). The session.cwd field is best-effort:
// some adapters omit it.
func currentCWD(env *envelope.Envelope) string {
	if env != nil && env.Session.CWD != "" {
		return env.Session.CWD
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

// commandFromOutcomeContext re-extracts the command string for the
// backup request. The matcher already parsed it once; we re-parse
// rather than thread it through Outcome so this stays a thin layer.
func commandFromOutcomeContext(env *envelope.Envelope) string {
	pl, ok := parsePayload(env)
	if !ok {
		return ""
	}
	if s, ok := pl.ToolInput["command"].(string); ok {
		return s
	}
	return ""
}
