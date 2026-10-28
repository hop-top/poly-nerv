package guardrails

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// loadCachedPacks loads + caches packs. First call walks up from
// argv[0] / cwd looking for spec/rulepacks/. Subsequent calls return
// the cached slice. Daemon restart re-loads.
//
// In test contexts, callers can bypass via LoadPacks directly.
func loadCachedPacks() ([]Pack, error) {
	packCache.once.Do(func() {
		dir, err := findRulepacksDir()
		if err != nil {
			packCache.err = err
			return
		}
		packs, err := LoadPacks(dir)
		packCache.packs = packs
		packCache.err = err
	})
	return packCache.packs, packCache.err
}

var packCache struct {
	once  sync.Once
	packs []Pack
	err   error
}

// findRulepacksDir walks up from cwd looking for spec/rulepacks/.
// Falls back to the canonical install path if env var
// NERV_RULEPACKS_DIR is set (testing + ops override).
func findRulepacksDir() (string, error) {
	if v := os.Getenv("NERV_RULEPACKS_DIR"); v != "" {
		return v, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, "spec", "rulepacks")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("guardrails: spec/rulepacks dir not found")
		}
		dir = parent
	}
}
