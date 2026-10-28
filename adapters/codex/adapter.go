package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	adapter "hop.top/nerv/adapters/internal"
)

const cliName adapter.CLIName = "codex-cli"

// Adapter implements HookAdapter for OpenAI Codex CLI.
type Adapter struct {
	shellMatcher *ShellMatcher
}

// New creates a Codex adapter.
func New() *Adapter {
	return &Adapter{
		shellMatcher: NewShellMatcher(),
	}
}

// CLI returns the canonical CLI name.
func (a *Adapter) CLI() adapter.CLIName { return cliName }

// Detect probes whether Codex CLI is installed and returns its metadata.
func (a *Adapter) Detect() (adapter.DetectResult, error) {
	path, err := exec.LookPath("codex")
	if err != nil {
		return adapter.DetectResult{Found: false}, nil
	}

	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return adapter.DetectResult{
			Found: true,
			Info:  adapter.CLIInfo{Name: cliName, Path: path},
		}, nil
	}

	version := strings.TrimSpace(string(out))

	return adapter.DetectResult{
		Found: true,
		Info: adapter.CLIInfo{
			Name:    cliName,
			Version: version,
			Path:    path,
		},
	}, nil
}

// Capabilities reports supported hook points.
func (a *Adapter) Capabilities() adapter.CapabilityMap {
	return adapter.CapabilityMap{
		PreToolUse:  true,
		PostToolUse: true,
		PreSession:  true,
		PostSession: true,
		Streaming:   false,
	}
}

// ConfigPaths returns the Codex hook config file paths in precedence order.
func (a *Adapter) ConfigPaths() []string {
	home, _ := os.UserHomeDir()
	return []string{
		".codex/hooks.json",
		filepath.Join(home, ".codex", "hooks.json"),
	}
}

// FeatureEnabled checks if codex_hooks=true is set in ~/.codex/config.toml.
func (a *Adapter) FeatureEnabled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	data, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		return false
	}

	// Simple TOML check — no full parser needed for a single flag.
	// Handles: codex_hooks = true, codex_hooks=true, trailing comments.
	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "codex_hooks") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			val := strings.TrimSpace(parts[1])
			// Strip inline comments.
			if idx := strings.Index(val, "#"); idx >= 0 {
				val = strings.TrimSpace(val[:idx])
			}
			// Handle quoted or unquoted booleans.
			val = strings.Trim(val, `"'`)
			return strings.EqualFold(val, "true")
		}
	}
	return false
}

// codexHooksConfig mirrors the Codex hooks.json schema.
type codexHooksConfig struct {
	Hooks map[string][]codexHookEntry `json:"hooks"`
}

// codexHookEntry is a single event entry in hooks.json.
type codexHookEntry struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []codexHook `json:"hooks"`
}

// codexHook is a single hook command definition.
type codexHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// MergeConfig merges nerv hook config into Codex's hooks.json format.
// The existing config is preserved; nerv entries are added/updated.
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	var cfg codexHooksConfig

	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &cfg); err != nil {
			return nil, fmt.Errorf("codex: parse existing config: %w", err)
		}
	}

	if cfg.Hooks == nil {
		cfg.Hooks = make(map[string][]codexHookEntry)
	}

	nervCommand := "nerv dispatch"

	// Register nerv handler for each native event.
	nativeEvents := []string{
		"SessionStart", "UserPromptSubmit", "Stop",
	}
	for _, event := range nativeEvents {
		cfg.Hooks[event] = mergeNervEntry(
			cfg.Hooks[event], "", nervCommand,
		)
	}

	// Register shell matcher for PreToolUse/PostToolUse.
	for _, event := range []string{"PreToolUse", "PostToolUse"} {
		cfg.Hooks[event] = mergeNervEntry(
			cfg.Hooks[event], "shell", nervCommand,
		)
	}

	return json.MarshalIndent(cfg, "", "  ")
}

// Translate converts a nerv envelope to Codex-native format.
// Codex uses the same JSON envelope shape as Claude Code.
func (a *Adapter) Translate(envelope []byte) ([]byte, error) {
	// Codex mirrors Claude's envelope schema — pass through.
	// Validate it's valid JSON.
	var raw json.RawMessage
	if err := json.Unmarshal(envelope, &raw); err != nil {
		return nil, fmt.Errorf("codex: invalid envelope JSON: %w", err)
	}
	return envelope, nil
}

// ShellMatcher returns the adapter's shell matcher for synthetic events.
func (a *Adapter) ShellMatcher() *ShellMatcher {
	return a.shellMatcher
}

// mergeNervEntry ensures a nerv hook entry exists for the given matcher
// without duplicating. Returns the updated entry list.
func mergeNervEntry(
	entries []codexHookEntry, matcher, command string,
) []codexHookEntry {
	nervHook := codexHook{Type: "command", Command: command}

	for i, entry := range entries {
		if entry.Matcher == matcher {
			// Check if nerv hook already exists.
			for _, h := range entry.Hooks {
				if h.Command == command {
					return entries
				}
			}
			entries[i].Hooks = append(entries[i].Hooks, nervHook)
			return entries
		}
	}

	return append(entries, codexHookEntry{
		Matcher: matcher,
		Hooks:   []codexHook{nervHook},
	})
}
