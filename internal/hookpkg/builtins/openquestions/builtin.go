// Package openquestions implements the open-questions detector built-in
// hook. Phase 1 skeleton — wires the package into the planned built-in
// registry contract and provides a no-op Handle that returns Allow.
//
// All detection, drift, state, inbox, and bus logic lands in follow-up
// changes.
package openquestions

import (
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// stubBuiltinDef mirrors the planned BuiltinDef contract from
// internal/hookpkg/builtins. Defined locally so this package
// compiles before the registry lands on main.
//
// TODO: swap stubBuiltinDef for builtins.BuiltinDef and
// StubRuntimeCfg for builtins.BuiltinRuntimeCfg once the registry
// package merges.
type stubBuiltinDef struct {
	Name          string
	SchemaVersion int
	Events        []string
	Func          func(env *envelope.Envelope, cfg StubRuntimeCfg) (merger.Decision, error)
}

// StubRuntimeCfg mirrors BuiltinRuntimeCfg from the planned registry.
// UserCfg holds the per-builtin map decoded from the user's hooks
// config (Extra inline map). StateDir and InboxDir are
// pre-resolved by the registry from kit/xdg.
//
// TODO: replace with builtins.BuiltinRuntimeCfg.
type StubRuntimeCfg struct {
	UserCfg  map[string]any
	StateDir string
	InboxDir string
}

// Def is the registry entry for the open-questions built-in. It is NOT
// auto-registered; wiring into builtins.All happens separately.
var Def = stubBuiltinDef{
	Name:          "open-questions",
	SchemaVersion: 1,
	Events: []string{
		"UserPromptSubmit",
		"PreToolUse",
		"CwdChanged",
		"Stop",
		"SessionEnd",
		"PreCompact",
	},
	Func: Handle,
}

// Handle is the per-event entry point. Skeleton returns Allow; real
// behaviour is filled in by downstream changes:
//
//   - state load/save (XDG_STATE_HOME/nerv/open-questions/<sid>.json + TTL)
//   - Tier 1 regex matchers (port from the research prototype)
//   - Tier 2 LLM fallback + per-turn cache
//   - improvement-gap log
//   - drift detection (assignment / keyword / subagent / cwd / lifecycle)
//   - inbox writer + kit/bus emit (inbox_message_added etc.)
func Handle(env *envelope.Envelope, cfg StubRuntimeCfg) (merger.Decision, error) {
	_ = env
	_ = cfg
	return merger.Decision{Action: merger.Allow}, nil
}
