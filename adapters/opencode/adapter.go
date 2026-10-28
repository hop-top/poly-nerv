package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"hop.top/nerv/adapters/internal"
)

const cliName internal.CLIName = "opencode"

// Adapter implements internal.HookAdapter for OpenCode CLI.
type Adapter struct {
	// ProjectDir is the root of the OpenCode project.
	// Plugin is written to <ProjectDir>/.opencode/plugins/nerv-bridge.ts.
	ProjectDir string

	// NervdPath is the absolute path to the nervd binary.
	NervdPath string
}

// CLI returns the canonical CLI name.
func (a *Adapter) CLI() internal.CLIName {
	return cliName
}

// Detect probes whether OpenCode is installed.
func (a *Adapter) Detect() (internal.DetectResult, error) {
	path, err := exec.LookPath("opencode")
	if err != nil {
		return internal.DetectResult{Found: false}, nil
	}

	// Attempt to get version.
	out, err := exec.Command(path, "--version").Output()
	version := "unknown"
	if err == nil {
		version = strings.TrimSpace(string(out))
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

// Capabilities reports what hook points OpenCode supports.
func (a *Adapter) Capabilities() internal.CapabilityMap {
	return internal.CapabilityMap{
		PreToolUse:  true,
		PostToolUse: true,
		PreSession:  true,
		PostSession: true,
		Streaming:   false,
	}
}

// ConfigPaths returns paths to config files nerv manages for OpenCode.
func (a *Adapter) ConfigPaths() []string {
	return []string{
		filepath.Join(a.ProjectDir, "opencode.json"),
		filepath.Join(a.ProjectDir, ".opencode", "plugins", "nerv-bridge.ts"),
	}
}

// MergeConfig merges the nerv marker into OpenCode's existing
// opencode.json (ConfigPaths()[0]), preserving every other key. The
// bridge plugin itself is a separate file written by Install; the
// merge still requires NervdPath so the marker is never recorded for a
// bridge that cannot be generated.
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	if _, err := GeneratePlugin(a.NervdPath); err != nil {
		return nil, fmt.Errorf("opencode: generate plugin: %w", err)
	}
	return mergePluginConfig(existing)
}

// Translate converts an OpenCode native event to a nerv envelope.
func (a *Adapter) Translate(nativeEvent []byte) ([]byte, error) {
	env, err := ToEnvelope(nativeEvent, "")
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// Install wires the nerv bridge plugin into the OpenCode project.
// Creates .opencode/plugins/ directory and writes nerv-bridge.ts.
// Also updates opencode.json to ensure plugin discovery if needed.
func (a *Adapter) Install() error {
	pluginDir := filepath.Join(a.ProjectDir, ".opencode", "plugins")
	if err := os.MkdirAll(pluginDir, 0o750); err != nil {
		return fmt.Errorf("opencode: create plugin dir: %w", err)
	}

	// Write the TS plugin.
	plugin, err := GeneratePlugin(a.NervdPath)
	if err != nil {
		return err
	}
	pluginPath := filepath.Join(pluginDir, "nerv-bridge.ts")
	if err := os.WriteFile(pluginPath, []byte(plugin), 0o640); err != nil {
		return fmt.Errorf("opencode: write plugin: %w", err)
	}

	// Ensure opencode.json exists with plugin dir configured.
	configPath := filepath.Join(a.ProjectDir, "opencode.json")
	if err := ensurePluginConfig(configPath); err != nil {
		return fmt.Errorf("opencode: ensure config: %w", err)
	}

	return nil
}

// ensurePluginConfig ensures opencode.json carries the nerv marker.
func ensurePluginConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	out, err := mergePluginConfig(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o640)
}

// mergePluginConfig returns opencode.json content with the nerv marker
// set and all other keys preserved. Empty input yields a new config.
func mergePluginConfig(existing []byte) ([]byte, error) {
	cfg := map[string]any{}
	if len(bytes.TrimSpace(existing)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(existing))
		dec.UseNumber() // keep large integers exact
		if err := dec.Decode(&cfg); err != nil {
			return nil, fmt.Errorf("parse opencode.json: %w", err)
		}
		if cfg == nil { // literal null
			cfg = map[string]any{}
		}
	}

	// OpenCode auto-discovers plugins in .opencode/plugins/.
	// We set a marker so nerv can detect its own install.
	nerv, _ := cfg["nerv"].(map[string]any)
	if nerv == nil {
		nerv = make(map[string]any)
	}
	nerv["installed"] = true
	cfg["nerv"] = nerv

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
