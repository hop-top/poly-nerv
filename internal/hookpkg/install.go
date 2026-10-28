package hookpkg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"hop.top/kit/go/core/xdg"

	"hop.top/nerv/internal/config"
)

// Dirs holds the resolved paths used during install/remove.
type Dirs struct {
	DataDir    string // $XDG_DATA_HOME/nerv/hooks/<name>
	ProjectDir string // .nerv/ (project root)
	HooksYAML  string // .nerv/hooks.yaml
	HooksLock  string // .nerv/hooks.lock
}

// ResolveDirs builds the directory layout for a given hook name.
func ResolveDirs(name string) (Dirs, error) {
	dataDir, err := xdg.DataDir("nerv")
	if err != nil {
		return Dirs{}, fmt.Errorf("resolve data dir: %w", err)
	}

	return Dirs{
		DataDir:    filepath.Join(dataDir, "hooks", name),
		ProjectDir: ".nerv",
		HooksYAML:  filepath.Join(".nerv", "hooks.yaml"),
		HooksLock:  filepath.Join(".nerv", "hooks.lock"),
	}, nil
}

// Install downloads/copies a hook handler and registers it in
// hooks.yaml + hooks.lock. source may be a URL or local path.
func Install(name, source string) error {
	dirs, err := ResolveDirs(name)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dirs.DataDir, 0o755); err != nil {
		return fmt.Errorf("create hook dir: %w", err)
	}
	if err := os.MkdirAll(dirs.ProjectDir, 0o755); err != nil {
		return fmt.Errorf("create project dir: %w", err)
	}

	// Fetch handler content.
	content, err := fetch(source)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", source, err)
	}

	// Write handler file.
	handlerPath := filepath.Join(dirs.DataDir, "handler")
	if err := os.WriteFile(handlerPath, content, 0o755); err != nil {
		return fmt.Errorf("write handler: %w", err)
	}

	// Compute sha256.
	hash := sha256.Sum256(content)
	shaHex := hex.EncodeToString(hash[:])

	// Register in hooks.yaml.
	if err := registerHook(dirs.HooksYAML, name, handlerPath); err != nil {
		return fmt.Errorf("register hook: %w", err)
	}

	// Update lockfile.
	lf, err := ReadLockfile(dirs.HooksLock)
	if err != nil {
		return fmt.Errorf("read lockfile: %w", err)
	}
	lf.Add(LockEntry{
		Name:      name,
		Version:   "1.0.0",
		Source:    source,
		SHA256:    shaHex,
		Installed: time.Now().UTC().Truncate(time.Second),
	})
	if err := WriteLockfile(dirs.HooksLock, lf); err != nil {
		return fmt.Errorf("write lockfile: %w", err)
	}

	return nil
}

// Remove unregisters a hook from hooks.yaml and lockfile.
// If purge is true, also deletes handler files from disk.
func Remove(name string, purge bool) error {
	dirs, err := ResolveDirs(name)
	if err != nil {
		return err
	}

	// Remove from hooks.yaml.
	if err := unregisterHook(dirs.HooksYAML, name); err != nil {
		return fmt.Errorf("unregister hook: %w", err)
	}

	// Remove from lockfile.
	lf, err := ReadLockfile(dirs.HooksLock)
	if err != nil {
		return fmt.Errorf("read lockfile: %w", err)
	}
	lf.Remove(name)
	if err := WriteLockfile(dirs.HooksLock, lf); err != nil {
		return fmt.Errorf("write lockfile: %w", err)
	}

	// Purge handler files.
	if purge {
		if err := os.RemoveAll(dirs.DataDir); err != nil {
			return fmt.Errorf("purge hook files: %w", err)
		}
	}

	return nil
}

// fetch retrieves content from a URL or local path.
func fetch(source string) ([]byte, error) {
	if strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "https://") {
		resp, err := http.Get(source) //nolint:gosec // user-supplied URL
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }() // body fully read below; close error irrelevant
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(source)
}

// registerHook adds or updates a hook entry in hooks.yaml.
func registerHook(yamlPath, name, handlerPath string) error {
	cfg, err := loadProjectHooks(yamlPath)
	if err != nil {
		return err
	}

	// Check if already registered; update run path.
	for i, h := range cfg.Hooks {
		if h.Name == name {
			cfg.Hooks[i].Run = handlerPath
			return writeProjectHooks(yamlPath, cfg)
		}
	}

	cfg.Hooks = append(cfg.Hooks, config.HookEntry{
		Name: name,
		On:   "tool_call",
		Run:  handlerPath,
	})

	return writeProjectHooks(yamlPath, cfg)
}

// unregisterHook removes a hook entry from hooks.yaml by name.
func unregisterHook(yamlPath, name string) error {
	cfg, err := loadProjectHooks(yamlPath)
	if err != nil {
		return err
	}

	filtered := cfg.Hooks[:0]
	for _, h := range cfg.Hooks {
		if h.Name != name {
			filtered = append(filtered, h)
		}
	}
	cfg.Hooks = filtered
	return writeProjectHooks(yamlPath, cfg)
}

// loadProjectHooks reads a single hooks.yaml file.
func loadProjectHooks(path string) (*config.HooksConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &config.HooksConfig{Version: "1"}, nil
		}
		return nil, err
	}
	var cfg config.HooksConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// writeProjectHooks writes hooks.yaml.
func writeProjectHooks(path string, cfg *config.HooksConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
