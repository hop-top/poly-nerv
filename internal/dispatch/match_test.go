package dispatch

import (
	"encoding/json"
	"testing"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/envelope"
)

// makeMatchEnvelope builds an envelope with event and optional tool_name.
func makeMatchEnvelope(event, toolName string) *envelope.Envelope {
	env := &envelope.Envelope{Event: event}
	if toolName != "" {
		payload, _ := json.Marshal(map[string]string{"tool_name": toolName})
		env.Payload = payload
	}
	return env
}

func TestMatch_EventLookup(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "h1", On: "pre_tool_use"},
		config.HookEntry{Name: "h2", On: "post_tool_use"},
	)

	m := NewMatcher()
	res := m.Match(cfg, makeMatchEnvelope("pre_tool_use", ""))

	if len(res.Hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(res.Hooks))
	}
	if res.Hooks[0].Name != "h1" {
		t.Errorf("expected hook h1, got %s", res.Hooks[0].Name)
	}
}

func TestMatch_NoMatch(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "h1", On: "pre_tool_use"},
	)

	m := NewMatcher()
	res := m.Match(cfg, makeMatchEnvelope("unknown_event", ""))

	if len(res.Hooks) != 0 {
		t.Fatalf("expected 0 hooks, got %d", len(res.Hooks))
	}
}

func TestMatch_RegexFilter(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "shell-hook", On: "pre_tool_use", Match: "Bash|shell"},
		config.HookEntry{Name: "catch-all", On: "pre_tool_use"},
	)

	m := NewMatcher()

	// Should match the regex hook + catch-all.
	res := m.Match(cfg, makeMatchEnvelope("pre_tool_use", "Bash"))
	if len(res.Hooks) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(res.Hooks))
	}

	// Should NOT match the regex hook, but catch-all passes.
	res = m.Match(cfg, makeMatchEnvelope("pre_tool_use", "Read"))
	if len(res.Hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(res.Hooks))
	}
	if res.Hooks[0].Name != "catch-all" {
		t.Errorf("expected catch-all, got %s", res.Hooks[0].Name)
	}
}

func TestMatch_NoRegex(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "a", On: "session_start"},
		config.HookEntry{Name: "b", On: "session_start"},
	)

	m := NewMatcher()
	res := m.Match(cfg, makeMatchEnvelope("session_start", ""))

	if len(res.Hooks) != 2 {
		t.Fatalf("expected 2 hooks (no regex = pass all), got %d", len(res.Hooks))
	}
}

func TestMatch_RegexCached(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "h1", On: "pre_tool_use", Match: "Bash|shell"},
	)

	m := NewMatcher()

	// Match twice with the same pattern.
	m.Match(cfg, makeMatchEnvelope("pre_tool_use", "Bash"))
	m.Match(cfg, makeMatchEnvelope("pre_tool_use", "shell"))

	if m.RegexCacheLen() != 1 {
		t.Errorf("expected 1 cached regex, got %d", m.RegexCacheLen())
	}
}

func TestMatch_InvalidRegex(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "bad", On: "pre_tool_use", Match: "[invalid"},
		config.HookEntry{Name: "good", On: "pre_tool_use"},
	)

	m := NewMatcher()
	res := m.Match(cfg, makeMatchEnvelope("pre_tool_use", "Bash"))

	// Bad regex skipped (fail-open), good hook still returned.
	if len(res.Hooks) != 1 {
		t.Fatalf("expected 1 hook (bad skipped), got %d", len(res.Hooks))
	}
	if res.Hooks[0].Name != "good" {
		t.Errorf("expected good, got %s", res.Hooks[0].Name)
	}
}

func TestMatch_RequireCapability(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "cap-hook", On: "pre_tool_use", RequireCapability: "redact"},
		config.HookEntry{Name: "no-cap", On: "pre_tool_use"},
	)

	m := NewMatcher()
	res := m.Match(cfg, makeMatchEnvelope("pre_tool_use", ""))

	// Hook with require_capability is always skipped for now.
	if len(res.Hooks) != 1 {
		t.Fatalf("expected 1 hook (cap skipped), got %d", len(res.Hooks))
	}
	if res.Hooks[0].Name != "no-cap" {
		t.Errorf("expected no-cap, got %s", res.Hooks[0].Name)
	}
}

func TestMatch_PreservesOrder(t *testing.T) {
	cfg := compileCfg(
		config.HookEntry{Name: "first", On: "pre_tool_use"},
		config.HookEntry{Name: "second", On: "pre_tool_use"},
		config.HookEntry{Name: "third", On: "pre_tool_use"},
	)

	m := NewMatcher()
	res := m.Match(cfg, makeMatchEnvelope("pre_tool_use", ""))

	if len(res.Hooks) != 3 {
		t.Fatalf("expected 3 hooks, got %d", len(res.Hooks))
	}
	expected := []string{"first", "second", "third"}
	for i, name := range expected {
		if res.Hooks[i].Name != name {
			t.Errorf("hook[%d]: expected %s, got %s", i, name, res.Hooks[i].Name)
		}
	}
}
