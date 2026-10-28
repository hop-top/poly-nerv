package disable

import (
	"hop.top/nerv/internal/config"
)

// sidecarVersion is the schema version emitted on first write.
// See spec/disabled.schema.json. Re-exported from config so existing
// disable-package tests keep their canonical reference while config
// owns the constant.
const sidecarVersion = config.DisabledSidecarVersion

// Sidecar is the in-memory shape of .nerv/disabled.yaml.
//
// Aliased to config.Disabled so the dispatch pipeline can consume
// the same struct without importing this package. The CLI write
// helpers below stay here.
type Sidecar = config.Disabled

// Binding is one disabled (hook, event) pair.
type Binding = config.DisabledBinding

// loadSidecar reads .nerv/disabled.yaml. Returns a zero-value sidecar
// (with version set) when the file is missing.
func loadSidecar(path string) (*Sidecar, error) {
	return config.LoadDisabled(path)
}

// saveSidecar writes the sidecar back to disk, creating parent
// directories as needed.
func saveSidecar(path string, sc *Sidecar) error {
	return config.SaveDisabled(path, sc)
}

// hasEvent reports membership in sc.Events.
func hasEvent(sc *Sidecar, name string) bool {
	return sc.HasEvent(name)
}

// hasBinding reports membership in sc.Bindings.
func hasBinding(sc *Sidecar, b Binding) bool {
	return sc.HasBinding(b.Hook, b.Event)
}

// addEvent appends if absent. Returns true when a change happened.
func addEvent(sc *Sidecar, name string) bool {
	if hasEvent(sc, name) {
		return false
	}
	sc.Events = append(sc.Events, name)
	return true
}

// removeEvent strips name if present. Returns true when a change happened.
func removeEvent(sc *Sidecar, name string) bool {
	for i, e := range sc.Events {
		if e == name {
			sc.Events = append(sc.Events[:i], sc.Events[i+1:]...)
			return true
		}
	}
	return false
}

// addBinding appends if absent. Returns true when a change happened.
func addBinding(sc *Sidecar, b Binding) bool {
	if hasBinding(sc, b) {
		return false
	}
	sc.Bindings = append(sc.Bindings, b)
	return true
}

// removeBinding strips b if present. Returns true when a change happened.
func removeBinding(sc *Sidecar, b Binding) bool {
	for i, x := range sc.Bindings {
		if x.Hook == b.Hook && x.Event == b.Event {
			sc.Bindings = append(sc.Bindings[:i], sc.Bindings[i+1:]...)
			return true
		}
	}
	return false
}
