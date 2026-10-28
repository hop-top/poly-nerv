package claude

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"hop.top/nerv/adapters/internal"
)

// Adapter implements internal.HookAdapter for Claude Code CLI.
type Adapter struct {
	// homeDir overrides $HOME for testing.
	homeDir string
}

// NewAdapter creates a Claude Code adapter. Pass homeDir="" to use
// the real home directory.
func NewAdapter(homeDir string) *Adapter {
	return &Adapter{homeDir: homeDir}
}

// CLI returns the canonical name of the Claude Code CLI.
func (a *Adapter) CLI() internal.CLIName {
	return "claude"
}

// Detect probes whether the claude binary is installed and returns
// its version info.
func (a *Adapter) Detect() (internal.DetectResult, error) {
	path, err := exec.LookPath("claude")
	if err != nil {
		return internal.DetectResult{Found: false}, nil
	}

	version, err := detectVersion(path)
	if err != nil {
		return internal.DetectResult{
			Found: true,
			Info: internal.CLIInfo{
				Name: "claude",
				Path: path,
			},
		}, nil
	}

	return internal.DetectResult{
		Found: true,
		Info: internal.CLIInfo{
			Name:    "claude",
			Version: version,
			Path:    path,
		},
	}, nil
}

// Capabilities reports which hook points Claude Code supports.
func (a *Adapter) Capabilities() internal.CapabilityMap {
	return internal.CapabilityMap{
		PreToolUse:  true,
		PostToolUse: true,
		PreSession:  true,
		PostSession: true,
		Streaming:   false,
	}
}

// ConfigPaths returns paths to Claude's settings.json files.
func (a *Adapter) ConfigPaths() []string {
	home := a.homeDir
	if home == "" {
		home = userHomeDir()
	}
	return []string{
		home + "/.claude/settings.json",
		".claude/settings.json",
	}
}

// MergeConfig merges nerv hooks into Claude's existing settings.json.
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	return mergeHooksIntoSettings(existing)
}

// Translate converts a nerv envelope to Claude-native format.
// For Claude, this is mostly a passthrough since Claude is the
// reference CLI.
func (a *Adapter) Translate(envelope []byte) ([]byte, error) {
	return envelope, nil
}

// detectVersion runs `claude --version` and parses the output.
func detectVersion(path string) (string, error) {
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("claude --version: %w", err)
	}
	// Output is typically "claude <version>" or just "<version>".
	version := strings.TrimSpace(string(out))
	if parts := strings.Fields(version); len(parts) > 1 {
		version = parts[len(parts)-1]
	}
	return version, nil
}

// userHomeDir returns the current user's home directory.
func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
