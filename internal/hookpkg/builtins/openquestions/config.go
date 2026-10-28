package openquestions

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// Default values per spec §"Toggle / config".
const (
	defaultLLMEnabled       = true
	defaultLLMProvider      = "nerv-primary"
	defaultLLMBudgetTokens  = 2000
	defaultStateTTLHours    = 168 // 7 days
	defaultInboxPathPattern = "$XDG_STATE_HOME/nerv/inbox/<agent_id>/open-questions/"
)

// defaultDriftKeywords is the initial pivot-marker list. Users can
// extend or replace via config; nil/empty in user config falls back
// to this list.
func defaultDriftKeywords() []string {
	return []string{
		"moving on",
		"let's switch",
		"different topic",
		"next:",
	}
}

// Config is the typed view of the open-questions section under
// hooks.builtin.open-questions in the user's nerv config.
type Config struct {
	LLM           LLMConfig `yaml:"llm"`
	DriftKeywords []string  `yaml:"drift_keywords"`
	StateTTLHours int       `yaml:"state_ttl_hours"`
	InboxPath     string    `yaml:"inbox_path"`
}

// LLMConfig captures the Tier 2 fallback knobs.
type LLMConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Provider     string `yaml:"provider"`
	BudgetTokens int    `yaml:"budget_tokens"`
}

// DecodeConfig converts a raw map (typically BuiltinRuntimeCfg.UserCfg)
// into a Config, applying defaults for any missing or zero fields.
//
// Round-trip: marshal raw → YAML → unmarshal into Config. This keeps
// the decode rules consistent with how the rest of nerv loads YAML
// config (gopkg.in/yaml.v3). nil raw is treated as "all defaults".
func DecodeConfig(raw map[string]any) (Config, error) {
	var cfg Config

	if len(raw) > 0 {
		buf, err := yaml.Marshal(raw)
		if err != nil {
			return Config{}, fmt.Errorf("openquestions: marshal raw config: %w", err)
		}
		if err := yaml.Unmarshal(buf, &cfg); err != nil {
			return Config{}, fmt.Errorf("openquestions: unmarshal config: %w", err)
		}
	}

	applyDefaults(&cfg, raw)
	return cfg, nil
}

// applyDefaults fills zero / missing fields. raw is consulted to
// distinguish "user explicitly set false / 0 / []" from "user omitted
// the key". Rules:
//
//   - LLM.Enabled defaults to true unless raw set llm.enabled
//   - LLM.Provider defaults to "nerv-primary" if empty
//   - LLM.BudgetTokens defaults to 2000 if <= 0
//   - StateTTLHours defaults to 168 if <= 0
//   - InboxPath defaults to the per-agent XDG path pattern if empty
//   - DriftKeywords defaults to defaultDriftKeywords() if nil and the
//     user did not set the key explicitly (set to []string{} to
//     intentionally disable keyword drift)
func applyDefaults(cfg *Config, raw map[string]any) {
	llmRaw, _ := raw["llm"].(map[string]any)

	if _, set := llmRaw["enabled"]; !set {
		cfg.LLM.Enabled = defaultLLMEnabled
	}
	if cfg.LLM.Provider == "" {
		cfg.LLM.Provider = defaultLLMProvider
	}
	if cfg.LLM.BudgetTokens <= 0 {
		cfg.LLM.BudgetTokens = defaultLLMBudgetTokens
	}

	if cfg.StateTTLHours <= 0 {
		cfg.StateTTLHours = defaultStateTTLHours
	}

	if cfg.InboxPath == "" {
		cfg.InboxPath = defaultInboxPathPattern
	}

	if _, set := raw["drift_keywords"]; !set {
		cfg.DriftKeywords = defaultDriftKeywords()
	}
}
