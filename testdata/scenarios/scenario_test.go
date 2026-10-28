package scenarios

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoad_BlockRmRf(t *testing.T) {
	s := Load(t, "01-block-rm-rf.json")
	if s.Name != "block-rm-rf" {
		t.Errorf("Name = %q; want block-rm-rf", s.Name)
	}
	if s.Expected.Action != "block" {
		t.Errorf("Expected.Action = %q; want block", s.Expected.Action)
	}
	if s.Envelope.Event != "PreToolUse" {
		t.Errorf("Envelope.Event = %q; want PreToolUse", s.Envelope.Event)
	}
}

func TestLoad_AllowLs(t *testing.T) {
	s := Load(t, "02-allow-ls.json")
	if s.Expected.Action != "allow" {
		t.Errorf("Expected.Action = %q; want allow", s.Expected.Action)
	}
}

func TestLoadAll(t *testing.T) {
	all := LoadAll(t)
	if len(all) < 15 {
		t.Errorf("LoadAll returned %d scenarios; want >= 15", len(all))
	}

	// Verify every scenario has required fields.
	for _, s := range all {
		if s.Name == "" {
			t.Error("scenario missing name")
		}
		if s.Envelope.Event == "" {
			t.Errorf("scenario %q: missing envelope.event", s.Name)
		}
		if s.Expected.Action == "" {
			t.Errorf("scenario %q: missing expected.action", s.Name)
		}
	}
}

func TestLoadAll_UniqueNames(t *testing.T) {
	all := LoadAll(t)
	seen := make(map[string]bool)
	for _, s := range all {
		if seen[s.Name] {
			t.Errorf("duplicate scenario name: %q", s.Name)
		}
		seen[s.Name] = true
	}
}

func TestScenario_HooksFixture(t *testing.T) {
	s := Load(t, "05-multi-hook-merge.json")
	if len(s.Hooks.Hooks) != 2 {
		t.Errorf("Hooks count = %d; want 2", len(s.Hooks.Hooks))
	}
}

func TestScenario_DaemonFixture(t *testing.T) {
	s := Load(t, "15-daemon-mode-fallback.json")
	if s.Daemon == nil {
		t.Fatal("Daemon fixture is nil; want non-nil")
	}
	if !s.Daemon.ExpectFallback {
		t.Error("Daemon.ExpectFallback = false; want true")
	}
}

func TestScenario_Synthesis(t *testing.T) {
	s := Load(t, "11-gemini-synthesis.json")
	if !s.Envelope.Synthesized {
		t.Error("Envelope.Synthesized = false; want true")
	}
	if s.Envelope.SynthesisSource == nil {
		t.Fatal("Envelope.SynthesisSource is nil; want non-nil")
	}
	if *s.Envelope.SynthesisSource != "BeforeTool" {
		t.Errorf(
			"SynthesisSource = %q; want BeforeTool",
			*s.Envelope.SynthesisSource,
		)
	}
}

// TestScenario_RedactionSecretExpanded checks the placeholder in
// 13-redaction-secrets.json loads as the full fake Stripe key, so the
// scenario still carries a token the redactor must catch.
func TestScenario_RedactionSecretExpanded(t *testing.T) {
	s := Load(t, "13-redaction-secrets.json")
	raw, err := json.Marshal(s.Envelope.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	want := "curl -H 'Authorization: Bearer " +
		"sk_" + "live_" + "abc123def456ghi789jkl012mno345" +
		"' https://api.example.com"
	if p.ToolInput.Command != want {
		t.Fatalf("command = %q, want %q", p.ToolInput.Command, want)
	}
	if strings.Contains(p.ToolInput.Command, "{{") {
		t.Fatalf("placeholder not expanded: %q", p.ToolInput.Command)
	}
}
