package dispatch

import (
	"encoding/json"
	"testing"
	"time"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/envelope"
	"pgregory.net/rapid"
)

// genHookEntry generates a random HookEntry with an event matching ev.
func genHookEntry(ev string) *rapid.Generator[config.HookEntry] {
	return rapid.Custom(func(t *rapid.T) config.HookEntry {
		h := config.HookEntry{
			Name: rapid.StringN(1, 30, 30).Draw(t, "hook-name"),
			On:   ev,
			Run:  "/bin/true",
		}
		// Occasionally add a regex match pattern (simple, safe patterns).
		if rapid.Bool().Draw(t, "has-match") {
			// Use literal prefixes to avoid regex DoS; no unbounded
			// quantifiers on variable input.
			h.Match = rapid.SampledFrom([]string{
				"^tool_",
				".*lint.*",
				"fmt$",
				"check",
				"[a-z]+",
				"^$",
			}).Draw(t, "match")
		}
		return h
	})
}

// genEnvelope generates a random Envelope with the given event.
func genEnvelope(ev string) *rapid.Generator[*envelope.Envelope] {
	return rapid.Custom(func(t *rapid.T) *envelope.Envelope {
		toolName := rapid.StringN(0, 40, 40).Draw(t, "tool-name")
		payload, _ := json.Marshal(map[string]string{"tool_name": toolName})
		return &envelope.Envelope{
			NervVersion: "1",
			Event:       ev,
			EventID:     "eid-1",
			Timestamp:   time.Now(),
			Payload:     payload,
		}
	})
}

func TestRapid_MatcherAlwaysTerminates(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		m := NewMatcher()
		ev := "pre_tool_use"

		nHooks := rapid.IntRange(1, 10).Draw(t, "n-hooks")
		hooks := make([]config.HookEntry, nHooks)
		for i := range hooks {
			hooks[i] = genHookEntry(ev).Draw(t, "hook")
		}

		cfg := config.Compile(&config.HooksConfig{
			Version: "1",
			Hooks:   hooks,
		})

		env := genEnvelope(ev).Draw(t, "env")

		done := make(chan *MatchResult, 1)
		go func() {
			done <- m.Match(cfg, env)
		}()

		select {
		case <-done:
			// ok — terminated
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Match did not terminate within 100ms")
		}
	})
}

func TestRapid_MatchReturnsSubset(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		m := NewMatcher()
		ev := "pre_tool_use"

		nHooks := rapid.IntRange(0, 15).Draw(t, "n-hooks")
		hooks := make([]config.HookEntry, nHooks)
		for i := range hooks {
			hooks[i] = genHookEntry(ev).Draw(t, "hook")
		}

		cfg := config.Compile(&config.HooksConfig{
			Version: "1",
			Hooks:   hooks,
		})

		env := genEnvelope(ev).Draw(t, "env")
		result := m.Match(cfg, env)

		if len(result.Hooks) > len(hooks) {
			t.Fatalf("matched %d hooks but config only has %d",
				len(result.Hooks), len(hooks))
		}
	})
}

func TestRapid_MatchPreservesOrder(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		m := NewMatcher()
		ev := "post_tool_use"

		nHooks := rapid.IntRange(2, 12).Draw(t, "n-hooks")
		hooks := make([]config.HookEntry, nHooks)
		for i := range hooks {
			hooks[i] = genHookEntry(ev).Draw(t, "hook")
			// Give each a unique name to track order.
			hooks[i].Name = rapid.StringN(1, 20, 20).Draw(t, "unique-name")
		}

		cfg := config.Compile(&config.HooksConfig{
			Version: "1",
			Hooks:   hooks,
		})

		env := genEnvelope(ev).Draw(t, "env")
		result := m.Match(cfg, env)

		// Verify output order matches config order.
		bucket := cfg.ByEvent[ev]
		ri := 0
		for _, bh := range bucket {
			if ri >= len(result.Hooks) {
				break
			}
			if bh.Name == result.Hooks[ri].Name {
				ri++
			}
		}
		if ri != len(result.Hooks) {
			t.Fatal("result order does not match config declaration order")
		}
	})
}
