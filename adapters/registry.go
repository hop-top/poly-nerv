// Package adapters provides the adapter registry and shared test
// infrastructure for nerv CLI adapters.
package adapters

import (
	"hop.top/nerv/adapters/aider"
	"hop.top/nerv/adapters/claude"
	"hop.top/nerv/adapters/codex"
	"hop.top/nerv/adapters/gemini"
	"hop.top/nerv/adapters/goose"
	"hop.top/nerv/adapters/internal"
	"hop.top/nerv/adapters/opencode"
	"hop.top/nerv/adapters/openhands"
	"hop.top/nerv/adapters/vibe"
)

// HookAdapter re-exports the internal interface for external use.
type HookAdapter = internal.HookAdapter

// Installer re-exports the transactional install framework for external
// use (e.g. the `nerv install` command).
type Installer = internal.Installer

// InstallRecord re-exports one install journal entry.
type InstallRecord = internal.InstallRecord

// NewInstaller re-exports the installer constructor.
var NewInstaller = internal.NewInstaller

// Registry returns the map of all registered CLI adapters keyed by
// canonical CLI name.
func Registry() map[string]HookAdapter {
	home := ""
	return map[string]HookAdapter{
		"aider":     &aider.Adapter{},
		"claude":    claude.NewAdapter(home),
		"codex":     codex.New(),
		"gemini":    &gemini.Adapter{},
		"goose":     &goose.Adapter{},
		"opencode":  &opencode.Adapter{},
		"openhands": &openhands.Adapter{},
		"vibe":      &vibe.Adapter{},
	}
}

// SupportedCLIs returns the sorted list of CLI names with adapters.
func SupportedCLIs() []string {
	return []string{
		"aider", "claude", "codex", "gemini", "goose",
		"opencode", "openhands", "vibe",
	}
}
