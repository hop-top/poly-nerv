package vibe

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	internal "hop.top/nerv/adapters/internal"
)

const cliName internal.CLIName = "vibe"

// Adapter implements HookAdapter for Mistral's Vibe CLI.
type Adapter struct{}

// CLI returns the canonical adapter name.
func (a *Adapter) CLI() internal.CLIName { return cliName }

// Detect probes whether Vibe CLI is installed.
func (a *Adapter) Detect() (internal.DetectResult, error) {
	p, err := exec.LookPath("vibe")
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

// Capabilities reports which hook points Vibe CLI supports.
func (a *Adapter) Capabilities() internal.CapabilityMap {
	return internal.CapabilityMap{
		PreToolUse:  true,
		PostToolUse: true,
		PreSession:  false,
		PostSession: false,
		Streaming:   false,
	}
}

// ConfigPaths returns paths to Vibe CLI hook config files.
func (a *Adapter) ConfigPaths() []string {
	return configPaths()
}

// MergeConfig merges nerv hook entries into existing Vibe config.
// Vibe uses TOML; we use simple string manipulation (no library).
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	content := string(existing)

	// If the config already has nerv handlers, return as-is.
	if strings.Contains(content, "nervd") {
		return existing, nil
	}

	// Build the hooks section with handlers for each native event.
	hooksSection := vibeHooksSection()

	// Append hooks section. If empty config, start fresh.
	if len(content) == 0 {
		return []byte(hooksSection), nil
	}

	// Check for existing [hooks] section and append handlers.
	if strings.Contains(content, "[hooks]") {
		return []byte(content + "\n" + vibeHandlersOnly()), nil
	}

	return []byte(content + "\n" + hooksSection), nil
}

// Translate converts a Vibe-native hook event to a nerv envelope.
func (a *Adapter) Translate(vibeEventBytes []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(vibeEventBytes, &m); err != nil {
		return nil, fmt.Errorf("vibe: parse event: %w", err)
	}
	env, err := ToEnvelope(m)
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// --- internal helpers ---

func configPaths() []string {
	paths := []string{filepath.Join(".vibe", "config.toml")}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append([]string{
			filepath.Join(home, ".vibe", "config.toml"),
		}, paths...)
	}
	return paths
}

// vibeHooksSection returns a full [hooks] TOML block.
func vibeHooksSection() string {
	return "[hooks]\nenabled = true\n" + vibeHandlersOnly()
}

// vibeHandlersOnly returns TOML handler entries for all native events.
func vibeHandlersOnly() string {
	events := []string{
		"UserMessageEvent",
		"ToolCallEvent",
		"ToolResultEvent",
		"CompactStartEvent",
		"CompactEndEvent",
		"AgentProfileChangedEvent",
	}

	var b strings.Builder
	for _, ev := range events {
		fmt.Fprintf(&b, "\n[[hooks.handlers]]\n")
		fmt.Fprintf(&b, "event = %q\n", ev)
		fmt.Fprintf(&b, "type = \"command\"\n")
		fmt.Fprintf(&b, "command = \"nervd\"\n")
		fmt.Fprintf(&b, "timeout = 30000\n")
	}
	return b.String()
}
