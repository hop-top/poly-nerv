//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildNervBinary compiles the nerv binary once per test into dir.
func buildNervBinary(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "nerv")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// writeFakeOpencode writes an `opencode` executable printing version.
func writeFakeOpencode(t *testing.T, binDir, version string) {
	t.Helper()
	script := "#!/bin/sh\necho '" + version + "'\n"
	if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

// runNervInstall runs `nerv install add opencode` in project, with an
// isolated HOME/XDG tree. Returns combined output and the run error.
func runNervInstall(t *testing.T, nerv, root, project string) (string, error) {
	t.Helper()
	cmd := exec.Command(nerv, "install", "add", "opencode")
	cmd.Dir = project
	cmd.Env = []string{
		"HOME=" + filepath.Join(root, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "home", ".config"),
		"XDG_DATA_HOME=" + filepath.Join(root, "home", ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(root, "home", ".local", "state"),
		"XDG_CACHE_HOME=" + filepath.Join(root, "home", ".cache"),
		"PATH=" + filepath.Join(root, "bin") + ":/usr/bin:/bin",
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func readTestFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func pathExists(p string) bool { _, err := os.Stat(p); return err == nil }

// TestInstallOpencode_E2E drives the real binary: every file the
// adapter needs is written, with the nervd command other adapters use,
// and the plugin layout follows the detected opencode version.
func TestInstallOpencode_E2E(t *testing.T) {
	root := t.TempDir()
	nerv := buildNervBinary(t, root)
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o750); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(root, "proj")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	v1Plugin := filepath.Join(project, ".opencode", "plugins", "nerv-bridge.ts")
	v2Plugin := filepath.Join(project, ".opencode", "nerv-bridge", "index.ts")
	cfgPath := filepath.Join(project, "opencode.json")

	t.Run("v1_writes_config_and_plugin", func(t *testing.T) {
		writeFakeOpencode(t, filepath.Join(root, "bin"), "1.19.3")
		if err := os.WriteFile(cfgPath, []byte(`{"theme":"dark"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := runNervInstall(t, nerv, root, project); err != nil {
			t.Fatalf("install: %v\n%s", err, out)
		}
		plugin := readTestFile(t, v1Plugin)
		if !strings.Contains(plugin, `const NERVD_PATH = "nervd";`) {
			t.Errorf("v1 plugin lacks nervd path:\n%.300s", plugin)
		}
		if !strings.Contains(plugin, `"tool.execute.before"`) {
			t.Errorf("v1 plugin is not the v1 bridge")
		}
		var cfg map[string]any
		if err := json.Unmarshal([]byte(readTestFile(t, cfgPath)), &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg["theme"] != "dark" {
			t.Errorf("user config key lost: %v", cfg)
		}
		if _, ok := cfg["plugins"]; ok {
			t.Errorf("v1 config must not carry a v2 plugins entry: %v", cfg)
		}
		if pathExists(v2Plugin) {
			t.Error("v2 plugin written for a v1 opencode")
		}
	})

	t.Run("upgrade_to_v2_replaces_v1_bridge", func(t *testing.T) {
		writeFakeOpencode(t, filepath.Join(root, "bin"), "opencode v2.2.5")
		if out, err := runNervInstall(t, nerv, root, project); err != nil {
			t.Fatalf("install: %v\n%s", err, out)
		}
		plugin := readTestFile(t, v2Plugin)
		for _, want := range []string{
			`const NERVD_PATH = "nervd";`,
			`id: "nerv.bridge"`,
			`ctx.tool.hook("execute.before"`,
			`ctx.tool.hook("execute.after"`,
			`ctx.session.hook("context"`,
			`ctx.event.subscribe(`,
		} {
			if !strings.Contains(plugin, want) {
				t.Errorf("v2 plugin missing %q", want)
			}
		}
		if pathExists(v1Plugin) {
			t.Error("stale v1 bridge left behind; v2 would auto-load it")
		}
		var cfg struct {
			Theme   string   `json:"theme"`
			Plugins []string `json:"plugins"`
			Nerv    struct {
				Installed bool `json:"installed"`
			} `json:"nerv"`
		}
		if err := json.Unmarshal([]byte(readTestFile(t, cfgPath)), &cfg); err != nil {
			t.Fatal(err)
		}
		if cfg.Theme != "dark" || !cfg.Nerv.Installed {
			t.Errorf("config = %+v", cfg)
		}
		if len(cfg.Plugins) != 1 || cfg.Plugins[0] != "./.opencode/nerv-bridge" {
			t.Errorf("plugins = %v, want [./.opencode/nerv-bridge]", cfg.Plugins)
		}
	})

	t.Run("v2_rerun_is_idempotent", func(t *testing.T) {
		before := readTestFile(t, cfgPath) + readTestFile(t, v2Plugin)
		if out, err := runNervInstall(t, nerv, root, project); err != nil {
			t.Fatalf("install: %v\n%s", err, out)
		}
		if after := readTestFile(t, cfgPath) + readTestFile(t, v2Plugin); after != before {
			t.Error("re-run changed installed files")
		}
		var cfg struct {
			Plugins []string `json:"plugins"`
		}
		_ = json.Unmarshal([]byte(readTestFile(t, cfgPath)), &cfg)
		if len(cfg.Plugins) != 1 {
			t.Errorf("plugins entry duplicated: %v", cfg.Plugins)
		}
	})

	t.Run("unknown_version_fails_without_writing", func(t *testing.T) {
		fresh := filepath.Join(root, "proj2")
		if err := os.MkdirAll(fresh, 0o750); err != nil {
			t.Fatal(err)
		}
		writeFakeOpencode(t, filepath.Join(root, "bin"), "garbage")
		out, err := runNervInstall(t, nerv, root, fresh)
		if err == nil {
			t.Fatalf("install succeeded for undetectable version:\n%s", out)
		}
		if pathExists(filepath.Join(fresh, "opencode.json")) || pathExists(filepath.Join(fresh, ".opencode")) {
			t.Error("files written despite failed install")
		}
	})
}
