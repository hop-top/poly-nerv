// Package scenarios provides types and loaders for golden test scenarios.
//
// Each scenario is a JSON file describing an end-to-end nerv test case:
// hooks config, envelope input, and expected decision output.
package scenarios

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeSecrets maps placeholders in scenario JSON to fake credentials
// assembled at runtime. The fixtures hold only the placeholder, so no
// checked-in file contains a credential-shaped literal that secret
// scanners (GitHub push protection and similar) would flag; the loaded
// scenario sees the full value.
var fakeSecrets = map[string]string{
	"{{FAKE_STRIPE_KEY}}": "sk_" + "live_" + "abc123def456ghi789jkl012mno345",
}

// expandFakeSecrets replaces every fakeSecrets placeholder in data.
func expandFakeSecrets(data []byte) []byte {
	s := string(data)
	for placeholder, value := range fakeSecrets {
		s = strings.ReplaceAll(s, placeholder, value)
	}
	return []byte(s)
}

// Scenario represents a single golden test case.
type Scenario struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Hooks       HooksFixture    `json:"hooks"`
	Envelope    EnvelopeFixture `json:"envelope"`
	Expected    Expected        `json:"expected"`
	Daemon      *DaemonFixture  `json:"daemon,omitempty"`
}

// HooksFixture is the hooks.yaml content for the scenario.
type HooksFixture struct {
	Version  string          `json:"version"`
	Defaults json.RawMessage `json:"defaults,omitempty"`
	Hooks    []HookFixture   `json:"hooks"`
}

// HookFixture is a single hook entry in the fixture.
type HookFixture struct {
	Name    string `json:"name"`
	On      string `json:"on"`
	Match   string `json:"match,omitempty"`
	Run     string `json:"run"`
	Timeout *int   `json:"timeout,omitempty"`
}

// EnvelopeFixture is the envelope input for the scenario.
type EnvelopeFixture struct {
	Event           string         `json:"event"`
	CLI             CLIFixture     `json:"cli"`
	Session         SessionFixture `json:"session"`
	Payload         any            `json:"payload"`
	Synthesized     bool           `json:"synthesized,omitempty"`
	SynthesisSource *string        `json:"synthesis_source,omitempty"`
}

// CLIFixture identifies the CLI in the fixture.
type CLIFixture struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// SessionFixture provides session context in the fixture.
type SessionFixture struct {
	ID string `json:"id"`
}

// Expected describes the expected decision outcome.
type Expected struct {
	Action           string         `json:"action"`
	MessageContains  string         `json:"message_contains,omitempty"`
	Rewrite          any            `json:"rewrite,omitempty"`
	MetadataContains map[string]any `json:"metadata_contains,omitempty"`
	RedactedFields   []string       `json:"redacted_fields,omitempty"`
	RedactionPattern string         `json:"redaction_pattern,omitempty"`
}

// DaemonFixture configures daemon-mode test behavior.
type DaemonFixture struct {
	Socket         string `json:"socket"`
	ExpectFallback bool   `json:"expect_fallback"`
}

// scenarioDir returns the absolute path to testdata/scenarios/.
func scenarioDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Dir(f)
}

// Load reads a single scenario JSON file by name (without path prefix).
func Load(t testing.TB, name string) *Scenario {
	t.Helper()
	path := filepath.Join(scenarioDir(), name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("scenarios.Load(%q): %v", name, err)
	}
	var s Scenario
	if err := json.Unmarshal(expandFakeSecrets(data), &s); err != nil {
		t.Fatalf("scenarios.Load(%q): unmarshal: %v", name, err)
	}
	return &s
}

// LoadAll reads every .json scenario from testdata/scenarios/.
func LoadAll(t testing.TB) []*Scenario {
	t.Helper()
	dir := scenarioDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("scenarios.LoadAll: %v", err)
	}
	var out []*Scenario
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		out = append(out, Load(t, e.Name()))
	}
	if len(out) == 0 {
		t.Fatal("scenarios.LoadAll: no scenarios found")
	}
	return out
}
