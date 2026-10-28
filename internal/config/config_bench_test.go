package config

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// benchHooksYAML generates a hooks.yaml with n hook entries.
func benchHooksYAML(n int) []byte {
	var buf []byte
	buf = append(buf, "version: \"1\"\ndefaults:\n  timeout: 5\n  on_error: continue\nhooks:\n"...)
	for i := range n {
		buf = append(buf, fmt.Sprintf(
			"  - name: hook-%d\n    on: pre-tool-use\n    match: \".*\"\n    run: /usr/bin/true\n    timeout: 10\n",
			i,
		)...)
	}
	return buf
}

// writeTempHooks writes a hooks.yaml fixture to a temp dir and returns
// the file path plus a cleanup function.
func writeTempHooks(b *testing.B, n int) (string, func()) {
	b.Helper()
	dir := b.TempDir()
	p := filepath.Join(dir, "hooks.yaml")
	if err := os.WriteFile(p, benchHooksYAML(n), 0o644); err != nil {
		b.Fatal(err)
	}
	return p, func() {}
}

// BenchmarkLoad measures hooks.yaml parse from disk (100 rules).
func BenchmarkLoad(b *testing.B) {
	p, cleanup := writeTempHooks(b, 100)
	defer cleanup()

	b.ResetTimer()
	for range b.N {
		if _, err := loadFromPaths([]string{p}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompile measures compile + index of a parsed config.
func BenchmarkCompile(b *testing.B) {
	p, cleanup := writeTempHooks(b, 100)
	defer cleanup()

	cfg, err := loadFromPaths([]string{p})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for range b.N {
		_ = Compile(cfg)
	}
}

// BenchmarkLoadCache measures gob-decode of a cached config.
func BenchmarkLoadCache(b *testing.B) {
	p, cleanup := writeTempHooks(b, 100)
	defer cleanup()

	cfg, err := loadFromPaths([]string{p})
	if err != nil {
		b.Fatal(err)
	}

	cc := Compile(cfg)
	cc.Version = "bench"
	cc.SourceMod = time.Now().Add(-time.Hour)

	cachePath := filepath.Join(b.TempDir(), "hooks.cache")
	if err := SaveCache(cc, cachePath); err != nil {
		b.Fatal(err)
	}

	// Verify it loads correctly before benchmarking.
	if got := LoadCache(cachePath, "bench", time.Now().Add(-2*time.Hour)); got == nil {
		b.Fatal("LoadCache returned nil for valid cache")
	}

	// Pre-register gob types (already done in init, but be explicit).
	gob.Register("")
	gob.Register([]any{})

	b.ResetTimer()
	for range b.N {
		_ = LoadCache(cachePath, "bench", time.Now().Add(-2*time.Hour))
	}
}
