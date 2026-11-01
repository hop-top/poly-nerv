package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIForVersion(t *testing.T) {
	cases := []struct {
		in   string
		want pluginAPI
		err  bool
	}{
		{"1.19.3", apiV1, false},
		{"1.19.5\n", apiV1, false},
		{"0.15.2", apiV1, false},
		{"opencode v2.2.5", apiV2, false},
		{"opencode v2.2.1", apiV2, false},
		{"2.0.0", apiV2, false},
		{"v3.0.0", 0, true},
		{"unknown", 0, true},
		{"", 0, true},
	}
	for _, c := range cases {
		got, err := apiForVersion(c.in)
		if (err != nil) != c.err {
			t.Errorf("apiForVersion(%q) err = %v, wantErr %v", c.in, err, c.err)
			continue
		}
		if got != c.want {
			t.Errorf("apiForVersion(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

// Detection shells out to `opencode --version`; both real output shapes
// must select the right generation.
func TestAdapter_PluginAPI_FromBinary(t *testing.T) {
	for _, c := range []struct {
		out  string
		want pluginAPI
	}{
		{"1.19.3", apiV1},
		{"opencode v2.2.5", apiV2},
	} {
		bin := t.TempDir()
		script := "#!/bin/sh\necho '" + c.out + "'\n"
		if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin)
		a := &Adapter{ProjectDir: t.TempDir(), NervdPath: "nervd"}
		got, err := a.pluginAPI()
		if err != nil || got != c.want {
			t.Errorf("%q: pluginAPI = %d, %v; want %d", c.out, got, err, c.want)
		}
	}
}

func TestAdapter_ConfigPaths_V2(t *testing.T) {
	a := &Adapter{ProjectDir: "/tmp/proj", Version: "opencode v2.2.5"}
	paths := a.ConfigPaths()
	if len(paths) != 2 || paths[1] != "/tmp/proj/.opencode/nerv-bridge/index.ts" {
		t.Errorf("ConfigPaths() = %v", paths)
	}
}

func TestAdapter_Install_V2(t *testing.T) {
	dir := t.TempDir()
	a := &Adapter{ProjectDir: dir, NervdPath: "nervd", Version: "2.2.5"}
	if err := a.Install(); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(dir, ".opencode", "nerv-bridge", "index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `const NERVD_PATH = "nervd";`) {
		t.Error("v2 bridge lacks nervd path")
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "opencode.json"))
	if !strings.Contains(string(cfg), `"./.opencode/nerv-bridge"`) {
		t.Errorf("plugins entry missing:\n%s", cfg)
	}
}

// A v1 -> v2 upgrade swaps the bridge in place: the stale v1 file would
// be auto-loaded by v2 and fail, so it must go; the reverse downgrades.
func TestAdapter_Install_SwitchesGenerations(t *testing.T) {
	dir := t.TempDir()
	v1 := filepath.Join(dir, ".opencode", "plugins", "nerv-bridge.ts")
	v2 := filepath.Join(dir, ".opencode", "nerv-bridge", "index.ts")
	install := func(version string) {
		t.Helper()
		a := &Adapter{ProjectDir: dir, NervdPath: "nervd", Version: version}
		if err := a.Install(); err != nil {
			t.Fatal(err)
		}
	}
	has := func(p string) bool { _, err := os.Stat(p); return err == nil }

	install("1.19.3")
	if !has(v1) || has(v2) {
		t.Fatalf("v1 install: v1=%v v2=%v", has(v1), has(v2))
	}
	install("2.2.5")
	if has(v1) || !has(v2) {
		t.Fatalf("upgrade: v1=%v v2=%v", has(v1), has(v2))
	}
	install("1.19.3")
	if !has(v1) || has(v2) {
		t.Fatalf("downgrade: v1=%v v2=%v", has(v1), has(v2))
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "opencode.json"))
	if strings.Contains(string(cfg), "nerv-bridge") {
		t.Errorf("downgrade left v2 plugins entry:\n%s", cfg)
	}
}

func TestMergePluginConfig_V2PreservesUserPlugins(t *testing.T) {
	in := []byte(`{"plugins":["a",{"package":"b"}]}`)
	out, err := mergePluginConfig(in, apiV2)
	if err != nil {
		t.Fatal(err)
	}
	again, err := mergePluginConfig(out, apiV2)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(again) {
		t.Errorf("not idempotent:\n%s\n%s", out, again)
	}
	s := string(out)
	if !strings.Contains(s, `"a"`) || !strings.Contains(s, `"package": "b"`) ||
		strings.Count(s, "./.opencode/nerv-bridge") != 1 {
		t.Errorf("plugins not merged cleanly:\n%s", s)
	}
}

func TestGeneratePluginV2(t *testing.T) {
	if _, err := GeneratePluginV2(""); err == nil {
		t.Error("empty nervd path accepted")
	}
	src, err := GeneratePluginV2(`/opt/we"ird\nervd`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, `const NERVD_PATH = "/opt/we\"ird\\nervd";`) {
		t.Errorf("nervd path not escaped:\n%.200s", src)
	}
}
