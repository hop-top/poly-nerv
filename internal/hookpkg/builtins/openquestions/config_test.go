package openquestions

import (
	"reflect"
	"testing"
)

func TestDecodeConfig_NilApplyAllDefaults(t *testing.T) {
	cfg, err := DecodeConfig(nil)
	if err != nil {
		t.Fatalf("DecodeConfig(nil) error: %v", err)
	}
	assertDefaults(t, cfg)
}

func TestDecodeConfig_EmptyMapAppliesAllDefaults(t *testing.T) {
	cfg, err := DecodeConfig(map[string]any{})
	if err != nil {
		t.Fatalf("DecodeConfig(empty) error: %v", err)
	}
	assertDefaults(t, cfg)
}

func TestDecodeConfig_FullRoundTrip(t *testing.T) {
	raw := map[string]any{
		"llm": map[string]any{
			"enabled":       false,
			"provider":      "claude",
			"budget_tokens": 4096,
		},
		"drift_keywords":  []any{"pivot", "switching gears"},
		"state_ttl_hours": 24,
		"inbox_path":      "/custom/inbox/path",
	}

	cfg, err := DecodeConfig(raw)
	if err != nil {
		t.Fatalf("DecodeConfig error: %v", err)
	}

	if cfg.LLM.Enabled {
		t.Errorf("LLM.Enabled = true, want false (explicitly set)")
	}
	if cfg.LLM.Provider != "claude" {
		t.Errorf("LLM.Provider = %q, want claude", cfg.LLM.Provider)
	}
	if cfg.LLM.BudgetTokens != 4096 {
		t.Errorf("LLM.BudgetTokens = %d, want 4096", cfg.LLM.BudgetTokens)
	}
	if cfg.StateTTLHours != 24 {
		t.Errorf("StateTTLHours = %d, want 24", cfg.StateTTLHours)
	}
	if cfg.InboxPath != "/custom/inbox/path" {
		t.Errorf("InboxPath = %q, want /custom/inbox/path", cfg.InboxPath)
	}
	want := []string{"pivot", "switching gears"}
	if !reflect.DeepEqual(cfg.DriftKeywords, want) {
		t.Errorf("DriftKeywords = %v, want %v", cfg.DriftKeywords, want)
	}
}

func TestDecodeConfig_PartialKeepsDefaults(t *testing.T) {
	raw := map[string]any{
		"state_ttl_hours": 48,
	}
	cfg, err := DecodeConfig(raw)
	if err != nil {
		t.Fatalf("DecodeConfig error: %v", err)
	}

	if cfg.StateTTLHours != 48 {
		t.Errorf("StateTTLHours = %d, want 48", cfg.StateTTLHours)
	}
	// Everything else should be defaults.
	if !cfg.LLM.Enabled {
		t.Errorf("LLM.Enabled = false, want true (default)")
	}
	if cfg.LLM.Provider != defaultLLMProvider {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, defaultLLMProvider)
	}
	if cfg.LLM.BudgetTokens != defaultLLMBudgetTokens {
		t.Errorf("LLM.BudgetTokens = %d, want %d", cfg.LLM.BudgetTokens, defaultLLMBudgetTokens)
	}
	if cfg.InboxPath != defaultInboxPathPattern {
		t.Errorf("InboxPath = %q, want %q", cfg.InboxPath, defaultInboxPathPattern)
	}
	if !reflect.DeepEqual(cfg.DriftKeywords, defaultDriftKeywords()) {
		t.Errorf("DriftKeywords = %v, want defaults %v", cfg.DriftKeywords, defaultDriftKeywords())
	}
}

func TestDecodeConfig_ExplicitFalseLLMRespected(t *testing.T) {
	raw := map[string]any{
		"llm": map[string]any{"enabled": false},
	}
	cfg, err := DecodeConfig(raw)
	if err != nil {
		t.Fatalf("DecodeConfig error: %v", err)
	}
	if cfg.LLM.Enabled {
		t.Fatal("LLM.Enabled = true; user explicitly set false, default must not override")
	}
}

func TestDecodeConfig_ZeroBudgetFallsBackToDefault(t *testing.T) {
	raw := map[string]any{
		"llm": map[string]any{"budget_tokens": 0},
	}
	cfg, err := DecodeConfig(raw)
	if err != nil {
		t.Fatalf("DecodeConfig error: %v", err)
	}
	if cfg.LLM.BudgetTokens != defaultLLMBudgetTokens {
		t.Errorf("BudgetTokens = %d, want default %d", cfg.LLM.BudgetTokens, defaultLLMBudgetTokens)
	}
}

func TestDecodeConfig_ExplicitEmptyDriftKeywordsRespected(t *testing.T) {
	// Setting drift_keywords to an empty list is a valid way to disable
	// keyword-based drift detection; defaults must NOT clobber it.
	raw := map[string]any{
		"drift_keywords": []any{},
	}
	cfg, err := DecodeConfig(raw)
	if err != nil {
		t.Fatalf("DecodeConfig error: %v", err)
	}
	if len(cfg.DriftKeywords) != 0 {
		t.Fatalf("DriftKeywords = %v, want empty (explicit)", cfg.DriftKeywords)
	}
}

func TestDecodeConfig_BadShapeReturnsError(t *testing.T) {
	// state_ttl_hours expects int; supply a string.
	raw := map[string]any{"state_ttl_hours": "not-a-number"}
	if _, err := DecodeConfig(raw); err == nil {
		t.Fatal("DecodeConfig accepted bad shape; want error")
	}
}

func assertDefaults(t *testing.T, cfg Config) {
	t.Helper()
	if !cfg.LLM.Enabled {
		t.Errorf("LLM.Enabled = false, want true (default)")
	}
	if cfg.LLM.Provider != defaultLLMProvider {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, defaultLLMProvider)
	}
	if cfg.LLM.BudgetTokens != defaultLLMBudgetTokens {
		t.Errorf("LLM.BudgetTokens = %d, want %d", cfg.LLM.BudgetTokens, defaultLLMBudgetTokens)
	}
	if cfg.StateTTLHours != defaultStateTTLHours {
		t.Errorf("StateTTLHours = %d, want %d", cfg.StateTTLHours, defaultStateTTLHours)
	}
	if cfg.InboxPath != defaultInboxPathPattern {
		t.Errorf("InboxPath = %q, want %q", cfg.InboxPath, defaultInboxPathPattern)
	}
	if !reflect.DeepEqual(cfg.DriftKeywords, defaultDriftKeywords()) {
		t.Errorf("DriftKeywords = %v, want %v", cfg.DriftKeywords, defaultDriftKeywords())
	}
}
