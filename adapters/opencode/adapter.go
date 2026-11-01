package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"hop.top/nerv/adapters/internal"
)

const cliName internal.CLIName = "opencode"

// Adapter implements internal.HookAdapter for OpenCode CLI.
type Adapter struct {
	// ProjectDir is the root of the OpenCode project; empty means the
	// current working directory. The bridge is written to
	// <ProjectDir>/.opencode/plugins/nerv-bridge.ts (plugin API v1) or
	// <ProjectDir>/.opencode/nerv-bridge/index.ts (plugin API v2).
	ProjectDir string

	// NervdPath is the nervd command the bridge runs: a bare name
	// resolved on PATH (the registry default, like the gemini and codex
	// adapters) or an absolute path.
	NervdPath string

	// Version pins the OpenCode version instead of asking
	// `opencode --version`. It selects the plugin API generation.
	Version string

	apiOnce sync.Once
	api     pluginAPI
	apiErr  error
}

// projectDir returns ProjectDir, defaulting to the working directory.
func (a *Adapter) projectDir() string {
	if a.ProjectDir != "" {
		return a.ProjectDir
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// pluginAPI resolves (once) which plugin API generation the installed
// OpenCode speaks, from Version or `opencode --version`.
func (a *Adapter) pluginAPI() (pluginAPI, error) {
	a.apiOnce.Do(func() {
		version := a.Version
		if version == "" {
			dr, err := a.Detect()
			if err != nil {
				a.apiErr = err
				return
			}
			version = dr.Info.Version
		}
		a.api, a.apiErr = apiForVersion(version)
	})
	return a.api, a.apiErr
}

// configPath, bridgePath and staleBridgePath locate the managed files.
func (a *Adapter) configPath() string {
	return filepath.Join(a.projectDir(), "opencode.json")
}

func bridgePath(dir string, api pluginAPI) string {
	if api == apiV2 {
		return filepath.Join(dir, ".opencode", "nerv-bridge", "index.ts")
	}
	return filepath.Join(dir, ".opencode", "plugins", "nerv-bridge.ts")
}

// v2PluginEntry is the opencode.json `plugins` entry (plugin API v2) for
// the bridge directory; relative entries resolve against the config.
const v2PluginEntry = "./.opencode/nerv-bridge"

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
// The bridge location follows the detected plugin API; when the version
// cannot be determined the v1 layout is reported (Install then fails).
func (a *Adapter) ConfigPaths() []string {
	api, err := a.pluginAPI()
	if err != nil {
		api = apiV1
	}
	return []string{a.configPath(), bridgePath(a.projectDir(), api)}
}

// MergeConfig merges the nerv marker (and, for plugin API v2, the
// bridge `plugins` entry) into OpenCode's existing opencode.json
// (ConfigPaths()[0]), preserving every other key. The bridge plugin
// itself is a separate file (see MergeFiles); the merge still requires
// NervdPath so the marker is never recorded for a bridge that cannot be
// generated.
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	api, err := a.pluginAPI()
	if err != nil {
		return nil, err
	}
	if _, err := a.generate(api); err != nil {
		return nil, fmt.Errorf("opencode: generate plugin: %w", err)
	}
	return mergePluginConfig(existing, api)
}

// generate renders the bridge source for api.
func (a *Adapter) generate(api pluginAPI) (string, error) {
	if api == apiV2 {
		return GeneratePluginV2(a.NervdPath)
	}
	return GeneratePlugin(a.NervdPath)
}

// MergeFiles implements internal.MultiFileAdapter: the merged
// opencode.json, the bridge for the detected plugin API, and removal of
// the other generation's bridge (a v2 OpenCode would auto-load a stale
// v1 plugin file and fail on it).
func (a *Adapter) MergeFiles(read internal.FileReader) (map[string][]byte, error) {
	api, err := a.pluginAPI()
	if err != nil {
		return nil, err
	}
	plugin, err := a.generate(api)
	if err != nil {
		return nil, fmt.Errorf("opencode: generate plugin: %w", err)
	}
	existing, _ := read(a.configPath())
	cfg, err := mergePluginConfig(existing, api)
	if err != nil {
		return nil, err
	}

	other := apiV1
	if api == apiV1 {
		other = apiV2
	}
	return map[string][]byte{
		a.configPath():                    cfg,
		bridgePath(a.projectDir(), api):   []byte(plugin),
		bridgePath(a.projectDir(), other): nil,
	}, nil
}

// Translate converts an OpenCode native event to a nerv envelope.
func (a *Adapter) Translate(nativeEvent []byte) ([]byte, error) {
	env, err := ToEnvelope(nativeEvent, "")
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// Install wires the nerv bridge into the OpenCode project: it writes
// the bridge for the detected plugin API, updates opencode.json and
// removes the other generation's bridge. The generic installer
// (internal.Installer) does the same transactionally via MergeFiles.
func (a *Adapter) Install() error {
	files, err := a.MergeFiles(func(path string) ([]byte, bool) {
		data, err := os.ReadFile(path)
		return data, err == nil
	})
	if err != nil {
		return err
	}
	for path, data := range files {
		if data == nil {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("opencode: remove %s: %w", path, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return fmt.Errorf("opencode: create dir: %w", err)
		}
		if err := os.WriteFile(path, data, 0o640); err != nil {
			return fmt.Errorf("opencode: write %s: %w", path, err)
		}
	}
	return nil
}

// mergePluginConfig returns opencode.json content with the nerv marker
// set and all other keys preserved. Empty input yields a new config.
// For plugin API v2 the bridge directory is added to `plugins` once; for
// v1 a bridge entry left by a v2 install is dropped (v1 auto-discovers
// .opencode/plugins/).
func mergePluginConfig(existing []byte, api pluginAPI) ([]byte, error) {
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

	// We set a marker so nerv can detect its own install.
	nerv, _ := cfg["nerv"].(map[string]any)
	if nerv == nil {
		nerv = make(map[string]any)
	}
	nerv["installed"] = true
	cfg["nerv"] = nerv

	plugins, _ := cfg["plugins"].([]any)
	kept := make([]any, 0, len(plugins)+1)
	has := false
	for _, p := range plugins {
		if s, ok := p.(string); ok && s == v2PluginEntry {
			if api == apiV1 || has {
				continue // dropped for v1; duplicates collapse for v2
			}
			has = true
		}
		kept = append(kept, p)
	}
	if api == apiV2 && !has {
		kept = append(kept, v2PluginEntry)
	}
	if len(kept) > 0 {
		cfg["plugins"] = kept
	} else {
		delete(cfg, "plugins")
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
