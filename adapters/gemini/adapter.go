package gemini

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	internal "hop.top/nerv/adapters/internal"
)

const cliName internal.CLIName = "gemini"

// Adapter implements HookAdapter for Gemini CLI.
type Adapter struct{}

// CLI returns the canonical adapter name.
func (a *Adapter) CLI() internal.CLIName { return cliName }

// Detect probes whether Gemini CLI is installed.
func (a *Adapter) Detect() (internal.DetectResult, error) {
	// Check common binary names.
	for _, name := range []string{"gemini", "gemini-cli"} {
		if p, err := findBinary(name); err == nil {
			return internal.DetectResult{
				Found: true,
				Info: internal.CLIInfo{
					Name: cliName,
					Path: p,
				},
			}, nil
		}
	}
	// Check config dirs as a fallback signal.
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

// Capabilities reports which hook points Gemini CLI supports.
func (a *Adapter) Capabilities() internal.CapabilityMap {
	return internal.CapabilityMap{
		PreToolUse:  true,
		PostToolUse: true,
		PreSession:  true,
		PostSession: true,
		Streaming:   false,
	}
}

// ConfigPaths returns paths to Gemini CLI hook config files.
func (a *Adapter) ConfigPaths() []string {
	return configPaths()
}

// MergeConfig merges nerv hook entries into existing Gemini settings.
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	var cfg map[string]any
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &cfg); err != nil {
			return nil, fmt.Errorf("gemini: parse settings: %w", err)
		}
	}
	if cfg == nil {
		cfg = make(map[string]any)
	}

	hooks, _ := cfg["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
	}

	// Register nerv handler for each Gemini native event.
	geminiEvents := []string{
		"SessionStart", "SessionEnd",
		"BeforeTool", "AfterTool",
		"BeforeAgent", "AfterAgent",
		"BeforeModel", "AfterModel",
		"Notification", "PreCompress",
		"BeforeToolSelection",
	}

	for _, ev := range geminiEvents {
		entry := nervHookEntry(ev)
		existing, _ := hooks[ev].([]any)
		upgradeLegacy(existing)
		if !hasNervEntry(existing) {
			hooks[ev] = append(existing, entry)
		}
	}

	cfg["hooks"] = hooks

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal settings: %w", err)
	}
	return out, nil
}

// Translate converts a Gemini-native hook event to a nerv envelope.
// Input: Gemini hook event JSON. Output: nerv envelope JSON.
func (a *Adapter) Translate(geminiEventBytes []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(geminiEventBytes, &m); err != nil {
		return nil, fmt.Errorf("gemini: parse event: %w", err)
	}
	env, err := ToEnvelope(m)
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// --- internal helpers ---

func configPaths() []string {
	paths := []string{filepath.Join(".gemini", "settings.json")}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append([]string{
			filepath.Join(home, ".gemini", "settings.json"),
		}, paths...)
	}
	return paths
}

// nervCommand is the hook command written for Gemini. --adapter makes
// nervd answer in Gemini's dialect (incl. decision context); Gemini's
// hook stdin carries hook_event_name, so no event argument is needed.
const nervCommand = "nervd --adapter gemini"

// legacyPrefix marks commands written before nervCommand existed
// ("nerv-adapter gemini <event>"), which resolve to no binary.
const legacyPrefix = "nerv-adapter gemini"

// upgradeLegacy rewrites legacy nerv commands in place.
func upgradeLegacy(hooks []any) {
	for _, h := range hooks {
		hm, _ := h.(map[string]any)
		inner, _ := hm["hooks"].([]any)
		for _, ih := range inner {
			ihm, _ := ih.(map[string]any)
			if c, _ := ihm["command"].(string); strings.HasPrefix(c, legacyPrefix) {
				ihm["command"] = nervCommand
			}
		}
	}
}

func nervHookEntry(event string) map[string]any {
	return map[string]any{
		"matcher":    ".*",
		"sequential": false,
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": nervCommand,
				"name":    "nerv-" + event,
				"timeout": 30000,
			},
		},
	}
}

func findBinary(name string) (string, error) {
	return exec.LookPath(name)
}

// hasNervEntry checks if a hook array already contains a nerv-managed entry.
func hasNervEntry(hooks []any) bool {
	for _, h := range hooks {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		inner, _ := hm["hooks"].([]any)
		for _, ih := range inner {
			ihm, ok := ih.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := ihm["type"].(string); t == "command" {
				if c, _ := ihm["command"].(string); c == nervCommand {
					return true
				}
			}
		}
	}
	return false
}
