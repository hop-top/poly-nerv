// Package config — disabled-sidecar loader.
//
// The .nerv/disabled.yaml sidecar holds event- and binding-level
// disable directives that complement the per-entry `disabled: true`
// flag in hooks.yaml. The schema lives at spec/disabled.schema.json.
//
// Ownership note: the writeable CLI primitives (DisableEvents,
// DisableBindings, EnableEvents, EnableBindings) live in
// internal/hookpkg/disable. The type + the read path are hoisted here
// so the dispatch pipeline can consume them without importing
// internal/hookpkg/disable (which would invert the dependency direction:
// disable already needs the wider config layer for HookEntry fixtures
// via its CLI surface).
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ErrNotProject signals that no .nerv/ project root was found walking
// up from the supplied start directory. Surfaced by FindProjectRoot
// and FindProjectRootFrom; the CLI disable package re-exports an alias
// so library callers can switch on it without importing config.
var ErrNotProject = errors.New("not inside a nerv project (no .nerv/ found from cwd upward)")

// FindProjectRoot walks up from the current working directory until
// it finds a .nerv/ directory. Returns the directory containing
// .nerv/, or ErrNotProject if no such ancestor exists before the
// filesystem root.
//
// Single source of truth for project-root discovery so the CLI
// (disable.OpenProject) and the daemon-side loader
// (LoadDisabledProject, future hooks.yaml layer walk-up) agree on
// where a sidecar lives regardless of the process's cwd at invocation.
func FindProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	return FindProjectRootFrom(cwd)
}

// FindProjectRootFrom is FindProjectRoot for an explicit starting
// directory. Useful for tests that want isolation from the real cwd.
func FindProjectRootFrom(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("abs %s: %w", start, err)
	}
	for {
		nervDir := filepath.Join(dir, ".nerv")
		if st, err := os.Stat(nervDir); err == nil && st.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotProject
		}
		dir = parent
	}
}

// DisabledSidecarVersion is the schema version emitted on first write.
// See spec/disabled.schema.json.
const DisabledSidecarVersion = "1.0-draft"

// Disabled is the in-memory shape of .nerv/disabled.yaml.
//
// A zero-value Disabled (no events, no bindings) is the "nothing
// disabled" semantic; loaders return it when the sidecar file is
// absent so callers can avoid nil-checks at the call site.
type Disabled struct {
	Version  string            `yaml:"version"`
	Events   []string          `yaml:"events,omitempty"`
	Bindings []DisabledBinding `yaml:"bindings,omitempty"`
}

// DisabledBinding is one disabled (hook, event) pair.
type DisabledBinding struct {
	Hook  string `yaml:"hook"`
	Event string `yaml:"event"`
}

// HasEvent reports membership in d.Events. Safe on a nil receiver.
func (d *Disabled) HasEvent(name string) bool {
	if d == nil {
		return false
	}
	for _, e := range d.Events {
		if e == name {
			return true
		}
	}
	return false
}

// HasBinding reports whether (hook, event) is in d.Bindings. Safe on
// a nil receiver.
func (d *Disabled) HasBinding(hook, event string) bool {
	if d == nil {
		return false
	}
	for _, b := range d.Bindings {
		if b.Hook == hook && b.Event == event {
			return true
		}
	}
	return false
}

// LoadDisabled reads .nerv/disabled.yaml from the supplied path.
//
// Returns a zero-value sidecar (with Version set) when the file is
// absent so callers can treat absence as an empty disabled-set
// without branching. Parse errors are surfaced; the caller decides
// whether to fail-open (typical) or hard-fail.
func LoadDisabled(path string) (*Disabled, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Disabled{Version: DisabledSidecarVersion}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var d Disabled
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if d.Version == "" {
		d.Version = DisabledSidecarVersion
	}
	return &d, nil
}

// SaveDisabled writes the sidecar back to disk, creating parent
// directories as needed. Canonical yaml.v3 emission; no comment
// preservation since the file is generated.
func SaveDisabled(path string, d *Disabled) error {
	if d.Version == "" {
		d.Version = DisabledSidecarVersion
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := yaml.Marshal(d)
	if err != nil {
		return fmt.Errorf("marshal disabled sidecar: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// DisabledSidecarPath returns .nerv/disabled.yaml under projectRoot.
func DisabledSidecarPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".nerv", "disabled.yaml")
}
