// Package hookpkg manages hook installation, removal, and lockfile state.
package hookpkg

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// LockEntry records a single installed hook.
type LockEntry struct {
	Name      string    `yaml:"name"`
	Version   string    `yaml:"version"`
	Source    string    `yaml:"source"`
	SHA256    string    `yaml:"sha256"`
	Installed time.Time `yaml:"installed"`
}

// Lockfile is the top-level structure for .nerv/hooks.lock.
type Lockfile struct {
	Hooks []LockEntry `yaml:"hooks"`
}

// ReadLockfile loads .nerv/hooks.lock from path. Returns empty
// Lockfile (not error) when file does not exist.
func ReadLockfile(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Lockfile{}, nil
		}
		return nil, err
	}
	var lf Lockfile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, err
	}
	return &lf, nil
}

// WriteLockfile writes the lockfile to path, creating parent dirs.
func WriteLockfile(path string, lf *Lockfile) error {
	data, err := yaml.Marshal(lf)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Add inserts or updates a lock entry by name.
func (lf *Lockfile) Add(e LockEntry) {
	for i, existing := range lf.Hooks {
		if existing.Name == e.Name {
			lf.Hooks[i] = e
			return
		}
	}
	lf.Hooks = append(lf.Hooks, e)
}

// Remove deletes a lock entry by name. Returns true if found.
func (lf *Lockfile) Remove(name string) bool {
	for i, e := range lf.Hooks {
		if e.Name == name {
			lf.Hooks = append(lf.Hooks[:i], lf.Hooks[i+1:]...)
			return true
		}
	}
	return false
}

// Has reports whether the lockfile contains an entry for name.
func (lf *Lockfile) Has(name string) bool {
	for _, e := range lf.Hooks {
		if e.Name == name {
			return true
		}
	}
	return false
}
