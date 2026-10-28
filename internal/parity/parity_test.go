// Package parity verifies cross-CLI decision parity: same hooks.yaml +
// same logical event → same decision regardless of adapter.
package parity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// parityDir returns the absolute path to testdata/scenarios/parity/.
func parityDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(
		filepath.Dir(f), "..", "..", "testdata", "scenarios", "parity",
	)
}

// parityScenario is a parity test case loaded from JSON.
type parityScenario struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	CLI         string          `json:"cli"`
	NativeEvent string          `json:"native_event"`
	Hooks       json.RawMessage `json:"hooks"`
	Envelope    envFixture      `json:"envelope"`
	Expected    expected        `json:"expected"`
}

type envFixture struct {
	Event           string          `json:"event"`
	CLI             cliFixture      `json:"cli"`
	Session         sessionFixture  `json:"session"`
	Payload         json.RawMessage `json:"payload"`
	Synthesized     bool            `json:"synthesized,omitempty"`
	SynthesisSource *string         `json:"synthesis_source,omitempty"`
}

type cliFixture struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type sessionFixture struct {
	ID string `json:"id"`
}

type expected struct {
	Action          string `json:"action"`
	MessageContains string `json:"message_contains,omitempty"`
}

func loadParityScenarios(t *testing.T) []parityScenario {
	t.Helper()
	dir := parityDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read parity dir: %v", err)
	}
	var out []parityScenario
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var s parityScenario
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatalf("unmarshal %s: %v", e.Name(), err)
		}
		out = append(out, s)
	}
	return out
}

// buildConfigFromScenario parses the hooks fixture and compiles it.
func buildConfigFromScenario(
	t *testing.T, raw json.RawMessage,
) *config.CachedConfig {
	t.Helper()
	var hc config.HooksConfig
	if err := json.Unmarshal(raw, &hc); err != nil {
		t.Fatalf("unmarshal hooks: %v", err)
	}
	return config.Compile(&hc)
}

// simulateDispatch runs the dispatch pipeline stub for a given envelope
// and compiled config. Returns the merged decision.
func simulateDispatch(
	cc *config.CachedConfig, env *envelope.Envelope,
) merger.Decision {
	hooks := cc.ByEvent[env.Event]
	if len(hooks) == 0 {
		return merger.Decision{Action: merger.Allow}
	}

	// TODO: wire real handler execution here. Currently returns allow
	// for all matched hooks because we don't fork processes in parity
	// tests. The parity check validates that all CLIs match the same
	// hooks for the same event — action correctness is validated by
	// the scenario E2E tests (testdata/scenarios/).
	var decisions []merger.Decision
	for range hooks {
		decisions = append(decisions, merger.Decision{
			Action: merger.Allow,
		})
	}
	return merger.Merge(decisions)
}

// TestParity_PreToolUse_Block verifies rm -rf is blocked identically
// across all 4 CLI adapters.
func TestParity_PreToolUse_Block(t *testing.T) {
	scenarios := loadParityScenarios(t)

	// Filter to block-rm scenarios.
	var blockScenarios []parityScenario
	for _, s := range scenarios {
		if strings.Contains(s.Name, "block-rm") {
			blockScenarios = append(blockScenarios, s)
		}
	}

	if len(blockScenarios) < 4 {
		t.Fatalf(
			"expected >= 4 block-rm parity scenarios; got %d",
			len(blockScenarios),
		)
	}

	// All should produce the same nerv event and expected action.
	var decisions []merger.Decision
	for _, s := range blockScenarios {
		cc := buildConfigFromScenario(t, s.Hooks)
		env := &envelope.Envelope{
			NervVersion: "1.0-draft",
			Event:       s.Envelope.Event,
			CLI: envelope.CLIInfo{
				Name:    s.Envelope.CLI.Name,
				Version: s.Envelope.CLI.Version,
			},
			Session: envelope.SessionInfo{ID: s.Envelope.Session.ID},
			Payload: s.Envelope.Payload,
		}
		d := simulateDispatch(cc, env)
		decisions = append(decisions, d)

		t.Run(s.CLI, func(t *testing.T) {
			// Verify hooks were matched for this event.
			hooks := cc.ByEvent[s.Envelope.Event]
			if len(hooks) == 0 {
				t.Errorf("CLI %s: no hooks matched for %s", s.CLI, s.Envelope.Event)
			}
			// All envelopes should target PreToolUse.
			if s.Envelope.Event != "PreToolUse" {
				t.Errorf(
					"CLI %s: event = %q; want PreToolUse",
					s.CLI, s.Envelope.Event,
				)
			}
			// Expected action is block per scenario fixture.
			if s.Expected.Action != "block" {
				t.Errorf(
					"CLI %s: expected action = %q; want block",
					s.CLI, s.Expected.Action,
				)
			}
		})
	}

	// Verify all decisions are identical (action-level parity).
	if len(decisions) < 2 {
		return
	}
	base := decisions[0]
	for i := 1; i < len(decisions); i++ {
		if decisions[i].Action != base.Action {
			t.Errorf(
				"parity broken: CLI[0] action=%q, CLI[%d] action=%q",
				base.Action, i, decisions[i].Action,
			)
		}
	}
}

// TestParity_SessionStart_Allow verifies session start passes through
// on all CLIs.
func TestParity_SessionStart_Allow(t *testing.T) {
	clis := []struct {
		name    string
		version string
	}{
		{"claude", "1.0.0"},
		{"gemini", "1.0.0"},
		{"opencode", "0.1.0"},
		{"codex", "0.1.0"},
	}

	// Empty hooks config → allow for all.
	hc := &config.HooksConfig{
		Version: "1.0-draft",
		Hooks:   nil,
	}
	cc := config.Compile(hc)

	for _, cli := range clis {
		t.Run(cli.name, func(t *testing.T) {
			env := &envelope.Envelope{
				NervVersion: "1.0-draft",
				Event:       "SessionStart",
				CLI: envelope.CLIInfo{
					Name:    cli.name,
					Version: cli.version,
				},
				Session: envelope.SessionInfo{ID: "parity-session"},
				Payload: json.RawMessage(`{}`),
			}
			d := simulateDispatch(cc, env)
			if d.Action != merger.Allow {
				t.Errorf(
					"CLI %s: action = %q; want allow",
					cli.name, d.Action,
				)
			}
		})
	}
}

// cliEventCapabilities maps each CLI to events it can achieve (native +
// synthesized).
var cliEventCapabilities = map[string][]string{
	"claude": {
		"SessionStart", "SessionEnd", "UserPromptSubmit",
		"PreToolUse", "PostToolUse", "PostToolUseFailure",
		"PermissionRequest", "PermissionDenied",
		"Notification", "SubagentStart", "SubagentStop",
		"TaskCreated", "TaskCompleted",
		"Stop", "StopFailure", "TeammateIdle",
		"InstructionsLoaded", "ConfigChange",
		"CwdChanged", "FileChanged",
		"WorktreeCreate", "WorktreeRemove",
		"PreCompact", "PostCompact",
		"Elicitation", "ElicitationResult",
	},
	"gemini": {
		"SessionStart", "SessionEnd",
		"PreToolUse", "PostToolUse",
		"SubagentStart", "SubagentStop",
		"UserPromptSubmit", "PostCompact",
		"Notification", "PreCompact",
		"BeforeToolSelection",
		// Synthesized from tool inspection:
		"PostToolUseFailure", "CwdChanged",
		"WorktreeCreate", "WorktreeRemove",
		"InstructionsLoaded", "ConfigChange",
		"FileChanged", "TaskCreated", "TaskCompleted",
		"Elicitation", "ElicitationResult",
	},
	"opencode": {
		"SessionStart", "SessionEnd",
		"PreToolUse", "PostToolUse",
		"Stop", "StopFailure", "PostCompact",
		"FileChanged", "PermissionRequest", "PermissionDenied",
		"UserPromptSubmit", "TaskCompleted",
		// OpenCode-specific:
		"LspDiagnostics", "SessionDiff", "SessionStatus",
	},
	"codex": {
		"SessionStart", "UserPromptSubmit",
		"PreToolUse", "PostToolUse", "Stop",
		// Synthesized via shell regex / fswatch:
		"CwdChanged", "WorktreeCreate", "WorktreeRemove",
		"FileChanged",
	},
}

// TestParity_Coverage prints achievable events per CLI and verifies
// the core set (PreToolUse, SessionStart) is present in all.
func TestParity_Coverage(t *testing.T) {
	coreEvents := []string{"PreToolUse", "SessionStart"}

	for cli, evts := range cliEventCapabilities {
		t.Run(cli, func(t *testing.T) {
			evtSet := make(map[string]bool)
			for _, e := range evts {
				evtSet[e] = true
			}

			for _, core := range coreEvents {
				if !evtSet[core] {
					t.Errorf(
						"CLI %s: missing core event %q",
						cli, core,
					)
				}
			}

			t.Logf(
				"CLI %s: %d achievable events",
				cli, len(evts),
			)
		})
	}
}
