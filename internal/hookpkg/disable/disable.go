// Package disable implements project-scoped enable/disable for nerv
// hooks, events, and hook×event bindings.
//
// Three units of granularity, two sidecar files:
//
//   - Hooks → `disabled: true` on the entry in .nerv/hooks.yaml.
//     Stripping the field re-enables.
//   - Events → top-level `events:` list in .nerv/disabled.yaml. Add
//     to disable, remove to enable.
//   - Bindings → top-level `bindings:` list in .nerv/disabled.yaml.
//     Each entry is `{hook, event}`. Finer granularity than the
//     per-hook flag or per-event list.
//
// All write paths are idempotent: disabling an already-disabled name
// is a no-op (no changed entry returned, no file write). Enable is
// symmetric.
//
// Package-level functions take exact names. Fuzzy matching + user
// confirmation live in the CLI layer (cmd/nerv/cmd_disable.go) so
// the library stays scriptable.
//
// Daemon enforcement of the disabled bits lands separately;
// this package only writes the configuration.
package disable

import (
	"path/filepath"

	"hop.top/nerv/internal/config"
)

// ErrNotProject signals that no .nerv/ project root was found from
// the current working directory upward. Aliased to config.ErrNotProject
// so the walk-up has one canonical sentinel — CLI callers and the
// daemon-side loader agree.
var ErrNotProject = config.ErrNotProject

// Project locates .nerv/ and operates on hooks.yaml + disabled.yaml
// within it. Construct with OpenProject (or OpenProjectAt for tests).
type Project struct {
	// Root is the path containing the .nerv/ directory.
	Root string
	// HooksYAML is filepath.Join(Root, ".nerv", "hooks.yaml").
	HooksYAML string
	// DisabledYAML is filepath.Join(Root, ".nerv", "disabled.yaml").
	DisabledYAML string
}

// OpenProject walks up from the current working directory until it
// finds a .nerv/ directory. Errors with ErrNotProject if none exists
// before the filesystem root.
//
// Delegates the walk-up to config.FindProjectRoot so the CLI and the
// daemon-side sidecar loader resolve to the same directory regardless
// of the process's cwd at invocation.
func OpenProject() (*Project, error) {
	root, err := config.FindProjectRoot()
	if err != nil {
		return nil, err
	}
	return projectAt(root), nil
}

// OpenProjectAt is OpenProject for an explicit starting directory.
// Useful for tests that want isolation from the real cwd.
func OpenProjectAt(start string) (*Project, error) {
	root, err := config.FindProjectRootFrom(start)
	if err != nil {
		return nil, err
	}
	return projectAt(root), nil
}

// projectAt builds a Project view rooted at the directory containing
// the .nerv/ marker.
func projectAt(root string) *Project {
	nervDir := filepath.Join(root, ".nerv")
	return &Project{
		Root:         root,
		HooksYAML:    filepath.Join(nervDir, "hooks.yaml"),
		DisabledYAML: filepath.Join(nervDir, "disabled.yaml"),
	}
}

// ListHookNames returns the names declared in hooks.yaml. Useful for
// fuzzy-match resolution at the CLI layer.
func (p *Project) ListHookNames() ([]string, error) {
	return listHookNames(p.HooksYAML)
}

// LoadSidecar returns the current .nerv/disabled.yaml (a zero-value
// Sidecar with the version set when the file is absent).
//
// Parse-error semantics differ from the daemon load path on purpose:
// the daemon (config.LoadDisabledProject) swallows parse errors and
// returns an empty sidecar so dispatch stays fail-open, but this CLI
// write path surfaces them verbatim. A malformed sidecar must fail
// the user-visible write so the operator sees the corruption
// immediately instead of silently overwriting a broken file.
func (p *Project) LoadSidecar() (*Sidecar, error) {
	return loadSidecar(p.DisabledYAML)
}

// DisableHooks sets `disabled: true` on each entry whose name appears
// in names. Idempotent: names already disabled contribute nothing.
//
// Returns:
//   - changed: hook names that flipped to disabled
//   - unknown: names that didn't match any hook in hooks.yaml
func (p *Project) DisableHooks(names []string) (changed, unknown []string, err error) {
	if len(names) == 0 {
		return nil, nil, nil
	}
	return setHookDisabled(p.HooksYAML, names, true)
}

// EnableHooks strips the `disabled:` field from each matching entry.
// Symmetric counterpart to DisableHooks; same return contract.
func (p *Project) EnableHooks(names []string) (changed, unknown []string, err error) {
	if len(names) == 0 {
		return nil, nil, nil
	}
	return dropHookDisabled(p.HooksYAML, names)
}

// DisableEvents adds each event to the sidecar's `events:` list.
// Idempotent. Returns the names that were freshly added (existing
// entries are skipped silently).
func (p *Project) DisableEvents(events []string) ([]string, error) {
	if len(events) == 0 {
		return nil, nil
	}
	sc, err := p.LoadSidecar()
	if err != nil {
		return nil, err
	}
	var added []string
	for _, ev := range events {
		if addEvent(sc, ev) {
			added = append(added, ev)
		}
	}
	if len(added) == 0 {
		return nil, nil
	}
	if err := saveSidecar(p.DisabledYAML, sc); err != nil {
		return nil, err
	}
	return added, nil
}

// EnableEvents removes each event from the sidecar. Returns the
// names that were actually stripped.
func (p *Project) EnableEvents(events []string) ([]string, error) {
	if len(events) == 0 {
		return nil, nil
	}
	sc, err := p.LoadSidecar()
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, ev := range events {
		if removeEvent(sc, ev) {
			removed = append(removed, ev)
		}
	}
	if len(removed) == 0 {
		return nil, nil
	}
	if err := saveSidecar(p.DisabledYAML, sc); err != nil {
		return nil, err
	}
	return removed, nil
}

// DisableBindings appends each (hook, event) pair to the sidecar's
// `bindings:` list. Idempotent.
func (p *Project) DisableBindings(pairs []Binding) ([]Binding, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	sc, err := p.LoadSidecar()
	if err != nil {
		return nil, err
	}
	var added []Binding
	for _, b := range pairs {
		if addBinding(sc, b) {
			added = append(added, b)
		}
	}
	if len(added) == 0 {
		return nil, nil
	}
	if err := saveSidecar(p.DisabledYAML, sc); err != nil {
		return nil, err
	}
	return added, nil
}

// EnableBindings removes each pair from the sidecar.
func (p *Project) EnableBindings(pairs []Binding) ([]Binding, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	sc, err := p.LoadSidecar()
	if err != nil {
		return nil, err
	}
	var removed []Binding
	for _, b := range pairs {
		if removeBinding(sc, b) {
			removed = append(removed, b)
		}
	}
	if len(removed) == 0 {
		return nil, nil
	}
	if err := saveSidecar(p.DisabledYAML, sc); err != nil {
		return nil, err
	}
	return removed, nil
}
