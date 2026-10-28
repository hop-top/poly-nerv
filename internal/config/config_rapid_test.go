package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"pgregory.net/rapid"
)

var knownEvents = []string{
	"PreToolUse", "PostToolUse",
	"SessionStart", "SessionEnd",
	"Notification", "SubagentStart",
	"UserPromptSubmit",
}

func genHookEntry() *rapid.Generator[HookEntry] {
	return rapid.Custom(func(t *rapid.T) HookEntry {
		nEvents := rapid.IntRange(1, 3).Draw(t, "n-events")
		events := make([]any, nEvents)
		for i := range events {
			events[i] = rapid.SampledFrom(knownEvents).Draw(t, "event")
		}

		var on any
		if nEvents == 1 {
			on = events[0]
		} else {
			on = events
		}

		return HookEntry{
			Name: rapid.StringN(1, 30, 30).Draw(t, "hook-name"),
			On:   on,
			Run:  "/bin/true",
		}
	})
}

func genHooksConfig() *rapid.Generator[*HooksConfig] {
	return rapid.Custom(func(t *rapid.T) *HooksConfig {
		nHooks := rapid.IntRange(1, 15).Draw(t, "n-hooks")
		hooks := make([]HookEntry, nHooks)
		for i := range hooks {
			hooks[i] = genHookEntry().Draw(t, "hook")
		}
		return &HooksConfig{
			Version: "1",
			Hooks:   hooks,
		}
	})
}

func TestRapid_CompileByEventKeysAreSubset(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := genHooksConfig().Draw(t, "cfg")
		cc := Compile(cfg)

		// Collect all event names declared across hooks.
		declared := make(map[string]bool)
		for _, h := range cfg.Hooks {
			for _, ev := range h.ResolveOn() {
				declared[ev] = true
			}
		}

		for key := range cc.ByEvent {
			if !declared[key] {
				t.Fatalf("ByEvent key %q not in declared events", key)
			}
		}
	})
}

func TestRapid_EveryHookInAtLeastOneBucket(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		cfg := genHooksConfig().Draw(t, "cfg")
		cc := Compile(cfg)

		for i, h := range cfg.Hooks {
			events := h.ResolveOn()
			if len(events) == 0 {
				continue
			}

			found := false
			for _, ev := range events {
				for _, bh := range cc.ByEvent[ev] {
					if bh.Name == h.Name && bh.Run == h.Run {
						found = true
						break
					}
				}
				if found {
					break
				}
			}

			if !found {
				t.Fatalf("hook[%d] %q not found in any ByEvent bucket",
					i, h.Name)
			}
		}
	})
}

func TestRapid_SaveLoadCacheRoundtrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		cfg := genHooksConfig().Draw(rt, "cfg")
		applyDefaults(cfg)
		cc := Compile(cfg)
		cc.Version = "test-v1"

		dir, err := os.MkdirTemp("", "rapid-cache-*")
		if err != nil {
			rt.Fatal(err)
		}
		defer func() { _ = os.RemoveAll(dir) }() // temp dir cleanup

		cachePath := filepath.Join(dir, "hooks.cache")

		if err := SaveCache(cc, cachePath); err != nil {
			rt.Fatalf("SaveCache: %v", err)
		}

		loaded := LoadCache(cachePath, "test-v1", cc.SourceMod)
		if loaded == nil {
			rt.Fatal("LoadCache returned nil")
		}

		// Compare ByEvent keys as sets.
		origKeys := mapKeySet(cc.ByEvent)
		loadKeys := mapKeySet(loaded.ByEvent)
		if !reflect.DeepEqual(origKeys, loadKeys) {
			rt.Fatal("ByEvent keys differ after roundtrip")
		}

		// Compare hook counts per event.
		for ev, hooks := range cc.ByEvent {
			if len(loaded.ByEvent[ev]) != len(hooks) {
				rt.Fatalf("event %q: got %d hooks, want %d",
					ev, len(loaded.ByEvent[ev]), len(hooks))
			}
		}

		// Compare config-level fields.
		if loaded.Config.Version != cc.Config.Version {
			rt.Fatalf("version mismatch: got %q, want %q",
				loaded.Config.Version, cc.Config.Version)
		}
	})
}

func mapKeySet(m map[string][]HookEntry) map[string]bool {
	s := make(map[string]bool, len(m))
	for k := range m {
		s[k] = true
	}
	return s
}
