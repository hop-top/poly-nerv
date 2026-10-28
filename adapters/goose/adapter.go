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
// Goose has rich native SSE events (Finish, Notification,
// UpdateConversation, permission, subagent, task events) plus
// synthesized events via the nerv bridge MCP extension.
func (a *Adapter) Capabilities() internal.CapabilityMap {
	return internal.CapabilityMap{
		PreToolUse:  true,
		PostToolUse: true,
		PreSession:  true,
		PostSession: true,
		Streaming:   true,
	}
}

// ConfigPaths returns paths to Goose config files.
func (a *Adapter) ConfigPaths() []string {
	return configPaths()
}

// MergeConfig merges nerv hook entries into existing Goose config.
// Goose uses YAML config. We output YAML via string manipulation
// — no yaml library.
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	content := string(existing)

	// Check if nerv extension is already configured.
	if strings.Contains(content, "nerv-bridge") {
		return existing, nil
	}

	if len(existing) == 0 {
		return []byte(nervExtensionBlock()), nil
	}

	// If extensions: key already exists, insert nerv-bridge under it
	// instead of appending a duplicate extensions: block.
	if strings.Contains(content, "extensions:") {
		return []byte(insertUnderExtensions(content)), nil
	}

	// No existing extensions: key — append full block.
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += "\n" + nervExtensionBlock()

	return []byte(content), nil
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

func configPaths() []string {
	paths := []string{
		filepath.Join(".goose", "config.yaml"),
	}
	if cfgDir, err := os.UserConfigDir(); err == nil {
		paths = append([]string{
			filepath.Join(cfgDir, "goose", "config.yaml"),
		}, paths...)
	}
	return paths
}

// nervExtensionBlock returns the full YAML config block including the
// extensions: key for use when no extensions key exists yet.
func nervExtensionBlock() string {
	return "# nerv hook integration\nextensions:\n" + nervExtensionEntry()
}

// nervExtensionEntry returns just the nerv-bridge entry (indented under
// extensions:) without the extensions: key itself.
func nervExtensionEntry() string {
	return `  nerv-bridge:
    type: stdio
    command: nerv
    args:
      - dispatch
      - goose
    enabled: true
`
}

// insertUnderExtensions inserts the nerv-bridge entry after the
// existing extensions: line, preserving all other content.
func insertUnderExtensions(content string) string {
	lines := strings.Split(content, "\n")
	var out []string
	inserted := false
	for _, line := range lines {
		out = append(out, line)
		if !inserted && strings.TrimSpace(line) == "extensions:" {
			// Insert nerv-bridge entry right after extensions:.
			entry := nervExtensionEntry()
			// entry already has trailing newline; split will add
			// an empty last element, trim it.
			entryLines := strings.Split(
				strings.TrimRight(entry, "\n"), "\n",
			)
			out = append(out, entryLines...)
			inserted = true
		}
	}
	return strings.Join(out, "\n")
}
