package goose

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

type capFile struct {
	Native []struct {
		Goose string `yaml:"goose"`
		Nerv  string `yaml:"nerv"`
	} `yaml:"native"`
	Close       []any    `yaml:"close"`
	Synthesized []any    `yaml:"synthesized"`
	Unsupported []string `yaml:"unsupported"`
}

func loadCaps(t *testing.T) capFile {
	t.Helper()
	data, err := os.ReadFile("capabilities.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var c capFile
	if err := yaml.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// Every native claim is a hook the installer writes and the decoder maps.
func TestCapabilities_NativeIsInstalledAndDecoded(t *testing.T) {
	c := loadCaps(t)
	installed := map[string]bool{}
	for _, ev := range installEvents {
		installed[ev] = true
	}
	claimed := map[string]bool{}
	for _, n := range c.Native {
		claimed[n.Goose] = true
		if !installed[n.Goose] {
			t.Errorf("native %s: not installed by MergeConfig", n.Goose)
		}
		got, ok := hookToNerv[n.Goose]
		if !ok || string(got) != n.Nerv {
			t.Errorf("native %s->%s: decoder maps to %q", n.Goose, n.Nerv, got)
		}
	}
	for ev := range installed {
		if !claimed[ev] {
			t.Errorf("installed hook %s missing from native", ev)
		}
	}
}

// No hook delivers synthesized or close events; nothing may claim them.
func TestCapabilities_NoSynthesizedOrClose(t *testing.T) {
	c := loadCaps(t)
	if len(c.Synthesized) != 0 || len(c.Close) != 0 {
		t.Errorf("synthesized=%d close=%d, want 0 (no delivered hook produces them)",
			len(c.Synthesized), len(c.Close))
	}
}

// native + unsupported partition the 26 adapter-scope events.
func TestCapabilities_Partition26(t *testing.T) {
	c := loadCaps(t)
	seen := map[string]int{}
	for _, n := range c.Native {
		seen[n.Nerv]++
	}
	for _, u := range c.Unsupported {
		seen[u]++
	}
	var dup []string
	for k, v := range seen {
		if v > 1 {
			dup = append(dup, k)
		}
	}
	sort.Strings(dup)
	if len(dup) > 0 {
		t.Errorf("events in both lists: %v", dup)
	}
	if len(seen) != 26 {
		t.Errorf("covered events = %d, want 26", len(seen))
	}
}

// Capabilities() must not claim streaming: the SSE bridge is gone.
func TestCapabilities_NoStreaming(t *testing.T) {
	b, _ := json.Marshal((&Adapter{}).Capabilities())
	var m map[string]bool
	_ = json.Unmarshal(b, &m)
	if m["streaming"] {
		t.Error("Streaming must be false: no SSE route is installed")
	}
}
