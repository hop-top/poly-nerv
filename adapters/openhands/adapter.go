package openhands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	internal "hop.top/nerv/adapters/internal"
)

const cliName internal.CLIName = "openhands"

// Adapter implements HookAdapter for OpenHands (formerly OpenDevin).
// OpenHands now uses a Claude Code-compatible hooks system (PR #1467).
type Adapter struct{}

// CLI returns the canonical adapter name.
func (a *Adapter) CLI() internal.CLIName { return cliName }

// Detect probes whether OpenHands is installed.
func (a *Adapter) Detect() (internal.DetectResult, error) {
	p, err := exec.LookPath("openhands")
	if err == nil {
		return internal.DetectResult{
			Found: true,
			Info: internal.CLIInfo{
				Name: cliName,
				Path: p,
			},
		}, nil
	}
	// Check config dirs as fallback signal.
	for _, cp := range configPaths() {
		if _, err := os.Stat(cp); err == nil {
			return internal.DetectResult{
				Found: true,
				Info:  internal.CLIInfo{Name: cliName},
			}, nil
		}
	}
	return internal.DetectResult{Found: false}, nil
}

// Capabilities reports which hook points OpenHands supports.
func (a *Adapter) Capabilities() internal.CapabilityMap {
	return internal.CapabilityMap{
		PreToolUse:  true,
		PostToolUse: true,
		PreSession:  true,
		PostSession: true,
		Streaming:   false,
	}
}

// ConfigPaths returns paths to OpenHands hook config files.
func (a *Adapter) ConfigPaths() []string {
	return configPaths()
}

// MergeConfig merges nerv hook entries into OpenHands hooks.json.
// OpenHands uses Claude Code-compatible JSON hooks format (PR #1467).
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	// Parse existing config preserving all fields via raw map.
	var raw map[string]json.RawMessage
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &raw); err != nil {
			return nil, fmt.Errorf(
				"openhands: parse hooks.json: %w", err,
			)
		}
	}
	if raw == nil {
		raw = make(map[string]json.RawMessage)
	}

	// Extract hooks sub-object.
	var cfg ohHooksConfig
	if h, ok := raw["hooks"]; ok {
		if err := json.Unmarshal(h, &cfg.Hooks); err != nil {
			return nil, fmt.Errorf(
				"openhands: parse hooks field: %w", err,
			)
		}
	}
	if cfg.Hooks == nil {
		cfg.Hooks = make(map[string][]ohHookEntry)
	}

	// Register nervd for all native hook events.
	nativeEvents := []string{
		"PreToolUse",
		"PostToolUse",
		"UserPromptSubmit",
		"SessionStart",
		"SessionEnd",
		"Stop",
	}

	for _, event := range nativeEvents {
		cfg.Hooks[event] = mergeNervEntry(cfg.Hooks[event])
	}

	// Marshal hooks back into the raw map to preserve other keys.
	hooksBytes, err := json.Marshal(cfg.Hooks)
	if err != nil {
		return nil, fmt.Errorf("openhands: marshal hooks: %w", err)
	}
	raw["hooks"] = hooksBytes

	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("openhands: marshal hooks.json: %w", err)
	}
	return out, nil
}

// Translate converts an OpenHands event to a nerv envelope.
func (a *Adapter) Translate(eventBytes []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(eventBytes, &m); err != nil {
		return nil, fmt.Errorf("openhands: parse event: %w", err)
	}
	env, err := ToEnvelope(m)
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// --- internal helpers ---

func configPaths() []string {
	paths := []string{
		filepath.Join(".openhands", "hooks.json"),
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append([]string{
			filepath.Join(home, ".openhands", "hooks.json"),
		}, paths...)
	}
	return paths
}

// ohHooksConfig mirrors the OpenHands hooks.json schema.
// Compatible with Claude Code's hooks format.
type ohHooksConfig struct {
	Hooks map[string][]ohHookEntry `json:"hooks"`
}

// ohHookEntry is a single event matcher entry in hooks.json.
type ohHookEntry struct {
	Matcher string   `json:"matcher"`
	Hooks   []ohHook `json:"hooks"`
}

// ohHook is a single hook command definition.
type ohHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// mergeNervEntry ensures a nerv hook entry exists without duplicating.
func mergeNervEntry(entries []ohHookEntry) []ohHookEntry {
	nervHook := ohHook{
		Type:    "command",
		Command: "nervd",
		Timeout: 30000,
	}

	// Check if nervd already registered.
	for _, entry := range entries {
		for _, h := range entry.Hooks {
			if h.Command == "nervd" {
				return entries
			}
		}
	}

	return append(entries, ohHookEntry{
		Matcher: ".*",
		Hooks:   []ohHook{nervHook},
	})
}
