package goose

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	internal "hop.top/nerv/adapters/internal"
)

const cliName internal.CLIName = "goose"

// Adapter implements HookAdapter for Goose CLI.
type Adapter struct{}

// CLI returns the canonical adapter name.
func (a *Adapter) CLI() internal.CLIName { return cliName }

// Detect probes whether Goose is installed.
func (a *Adapter) Detect() (internal.DetectResult, error) {
	path, err := exec.LookPath("goose")
	if err != nil {
		// Check config dirs as fallback signal.
		for _, cp := range detectPaths() {
			if _, err := os.Stat(cp); err == nil {
				return internal.DetectResult{
					Found: true,
					Info:  internal.CLIInfo{Name: cliName},
				}, nil
			}
		}
		return internal.DetectResult{Found: false}, nil
	}

	// Try both --version and version subcommand.
	version := ""
	for _, args := range [][]string{{"--version"}, {"version"}} {
		out, err := exec.Command(path, args...).Output()
		if err == nil {
			version = strings.TrimSpace(string(out))
			break
		}
	}

	return internal.DetectResult{
		Found: true,
		Info: internal.CLIInfo{
			Name:    cliName,
			Version: version,
			Path:    path,
		},
	}, nil
}

// Capabilities reports which hook points Goose supports.
// Tool, session, prompt and stop events arrive natively through plugin
// hooks (hooks.json); block is honored on PreToolUse and Stop only. No
// streaming: no SSE route is installed.
func (a *Adapter) Capabilities() internal.CapabilityMap {
	return internal.CapabilityMap{
		PreToolUse:  true,
		PostToolUse: true,
		PreSession:  true,
		PostSession: true,
	}
}

// ConfigPaths returns the hooks.json files nerv manages. Goose
// discovers hooks from plugin directories (user: ~/.agents/plugins/<name>/,
// project: <project>/.agents/plugins/<name>/); each plugin's
// hooks/hooks.json holds the rules. The user-scope file comes first and is
// the install target.
func (a *Adapter) ConfigPaths() []string {
	return configPaths()
}

// MergeConfig merges nerv hook rules into an existing plugin hooks.json.
// Empty input yields a fresh document. Existing nerv commands, including
// the legacy `nerv dispatch goose` and bare `nervd`, are rewritten to the
// adapter-routed command in place; other hooks and top-level keys are
// preserved. Output is deterministic, and merging its own output is a no-op.
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	doc := map[string]any{}
	if len(strings.TrimSpace(string(existing))) > 0 {
		if err := json.Unmarshal(existing, &doc); err != nil {
			return nil, fmt.Errorf("goose: existing hooks.json is not valid JSON: %w", err)
		}
	}
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for _, ev := range installEvents {
		rules, _ := hooks[ev].([]any)
		if !upgradeLegacy(rules) {
			rules = append(rules, map[string]any{
				"hooks": []any{map[string]any{
					"type":    "command",
					"command": nervCommand,
				}},
			})
		}
		hooks[ev] = rules
	}
	doc["hooks"] = hooks
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("goose: marshal hooks.json: %w", err)
	}
	return append(out, '\n'), nil
}

// upgradeLegacy rewrites legacy nerv commands in rules to nervCommand and
// reports whether any rule now carries it.
func upgradeLegacy(rules []any) bool {
	found := false
	for _, r := range rules {
		rule, _ := r.(map[string]any)
		actions, _ := rule["hooks"].([]any)
		for _, a := range actions {
			act, _ := a.(map[string]any)
			cmd, _ := act["command"].(string)
			switch strings.TrimSpace(cmd) {
			case nervCommand:
				found = true
			case "nervd", "nerv dispatch goose":
				act["command"] = nervCommand
				found = true
			}
		}
	}
	return found
}

// Translate converts a Goose-native event to a nerv envelope.
func (a *Adapter) Translate(eventBytes []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(eventBytes, &m); err != nil {
		return nil, fmt.Errorf("goose: parse event: %w", err)
	}
	env, err := ToEnvelope(m)
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// --- internal helpers ---

// nervCommand is the hook command written into Goose's hooks.json.
// --adapter makes nervd decode Goose's payload and answer in its dialect.
const nervCommand = "nervd --adapter goose"

// installEvents are the Goose hook events nerv subscribes to. Block is
// honored on PreToolUse and Stop only; the rest are observation. The
// Before*/After* shell and read events duplicate the tool events, and
// PreToolUseResult is observation-only, so none are installed.
var installEvents = []string{
	"SessionStart", "SessionEnd", "Stop", "UserPromptSubmit",
	"PreToolUse", "PostToolUse", "PostToolUseFailure", "AfterFileEdit",
}

func configPaths() []string {
	rel := filepath.Join(".agents", "plugins", "nerv", "hooks", "hooks.json")
	var paths []string
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, rel))
	}
	return append(paths, rel)
}

// detectPaths are presence signals used when the goose binary is not on
// PATH: goose's config file locations.
func detectPaths() []string {
	var paths []string
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "goose", "config.yaml"))
	}
	if cfgDir, err := os.UserConfigDir(); err == nil {
		paths = append(paths, filepath.Join(cfgDir, "goose", "config.yaml"))
	}
	return append(paths, filepath.Join(".goose", "config.yaml"))
}
