// Package builtins is the compile-time registry of in-process hook
// handlers. Built-ins ship inside the nerv binary (no external script
// install) and dispatch in-process via [BuiltinFunc] rather than
// fork/exec. See docs/architecture/builtins-registry.md for the full
// design.
//
// This package intentionally has no built-ins registered yet; entries
// are appended to [All] in later changes (e.g. open-questions hook). The dispatch
// path that consumes [Inject]'d entries lands separately.
package builtins

import (
	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// BuiltinFunc is the in-process handler signature. Same contract as
// the exec handler: receives the envelope, returns a merger decision.
// Panics are caught by the dispatch caller (callBuiltinSafe), which
// converts them to fail-open Allow decisions.
type BuiltinFunc func(env *envelope.Envelope, cfg BuiltinRuntimeCfg) (merger.Decision, error)

// BuiltinRuntimeCfg carries per-call deps for a built-in handler.
//
// UserCfg is the raw inline map under hooks.builtin.<name> (post-merge
// across config layers); each built-in decodes this map into its own
// strongly-typed Config struct. StateDir is the resolved
// $HOME/.nerv/state/<name>/ path. InboxDir is the resolved inbox path
// (from cfg or default).
//
// Bus reference will be added in a later task once kit-bus lands; it
// is deliberately omitted here to avoid cross-branch coupling.
type BuiltinRuntimeCfg struct {
	UserCfg  map[string]any
	StateDir string
	InboxDir string
}

// BuiltinDef declares a compiled-in hook. Each new built-in adds a
// single entry to [All] from its own subpackage.
type BuiltinDef struct {
	Name          string
	SchemaVersion int      // bump on breaking config/state schema change
	Events        []string // events this built-in subscribes to
	Func          BuiltinFunc
	Migrator      Migrator // nil = no migration
}

// Migrator upgrades stored state when a built-in's [BuiltinDef.SchemaVersion]
// is bumped past the value last recorded in
// [config.CachedConfig.BuiltinVersions].
type Migrator interface {
	Migrate(fromVersion int, stateDir string) error
}

// All is the compile-time registry. Built-ins register here from their
// own subpackages (e.g. openquestions.Def). Order is execution order
// when multiple built-ins subscribe to the same event.
var All = []BuiltinDef{}

// ByName returns the [BuiltinDef] registered under name and a boolean
// indicating whether it was found.
func ByName(name string) (BuiltinDef, bool) {
	for _, d := range All {
		if d.Name == name {
			return d, true
		}
	}
	return BuiltinDef{}, false
}

// Inject appends a synthetic [config.HookEntry] for every built-in into
// byEvent, one per event the built-in subscribes to. Entries land AFTER
// any user hooks already indexed for the same event (built-ins act as
// trailing observers per docs/architecture/builtins-registry.md §2).
//
// The synthetic entry carries Source="builtin" and Name="builtin:<name>";
// Run is left empty because dispatch routes by Source, not by Run.
//
// Inject is a no-op when [All] is empty.
//
// cfg is currently unused but kept in the signature so future
// per-builtin gating (e.g. honour cfg.Builtin[name].Enabled here rather
// than at dispatch time) can be added without breaking callers. Wired
// into [config.Compile].
func Inject(cfg *config.HooksConfig, byEvent map[string][]config.HookEntry) {
	_ = cfg // reserved for future gating; see doc comment
	for _, def := range All {
		for _, ev := range def.Events {
			byEvent[ev] = append(byEvent[ev], config.HookEntry{
				Name:   "builtin:" + def.Name,
				On:     ev,
				Source: "builtin",
			})
		}
	}
}
