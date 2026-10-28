package codex

import (
	"bytes"
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
// The existing config is preserved verbatim, including keys and fields
// nerv does not model; nerv entries are added without duplication.
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	cfg := map[string]any{}
	if len(bytes.TrimSpace(existing)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(existing))
		dec.UseNumber() // keep large integers exact
		if err := dec.Decode(&cfg); err != nil {
			return nil, fmt.Errorf("codex: parse existing config: %w", err)
		}
		if cfg == nil { // literal null
			cfg = map[string]any{}
		}
	}

	hooks := map[string]any{}
	if raw, ok := cfg["hooks"]; ok && raw != nil {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("codex: \"hooks\" must be an object")
		}
		hooks = m
	}

	nervCommand := "nerv dispatch"

	// Register nerv handler for each native event, then the shell
	// matcher for PreToolUse/PostToolUse.
	registrations := []struct{ event, matcher string }{
		{"SessionStart", ""},
		{"UserPromptSubmit", ""},
		{"Stop", ""},
		{"PreToolUse", "shell"},
		{"PostToolUse", "shell"},
	}
	for _, r := range registrations {
		entries, err := mergeNervEntry(
			hooks[r.event], r.matcher, nervCommand,
		)
		if err != nil {
			return nil, fmt.Errorf("codex: hooks.%s: %w", r.event, err)
		}
		hooks[r.event] = entries
	}
	cfg["hooks"] = hooks

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
// without duplicating. raw is the decoded event list (nil when absent);
// unknown fields on entries and hooks are left untouched.
func mergeNervEntry(raw any, matcher, command string) ([]any, error) {
	var entries []any
	if raw != nil {
		list, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("must be an array")
		}
		entries = list
	}

	nervHook := map[string]any{"type": "command", "command": command}

	for i, e := range entries {
		entry, ok := e.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("entry %d must be an object", i)
		}
		m, _ := entry["matcher"].(string)
		if m != matcher {
			continue
		}
		var list []any
		if h, ok := entry["hooks"]; ok && h != nil {
			if list, ok = h.([]any); !ok {
				return nil, fmt.Errorf("entry %d hooks must be an array", i)
			}
		}
		for _, h := range list {
			if hm, ok := h.(map[string]any); ok && hm["command"] == command {
				return entries, nil
			}
		}
		entry["hooks"] = append(list, nervHook)
		return entries, nil
	}

	entry := map[string]any{"hooks": []any{nervHook}}
	if matcher != "" {
		entry["matcher"] = matcher
	}
	return append(entries, entry), nil
}
