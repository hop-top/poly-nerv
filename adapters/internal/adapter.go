// Package internal defines the shared adapter interface and install framework
// for nerv CLI adapters.
//
// HookAdapter extends the base uxp.Adapter contract (hop.top/kit/uxp) with
// nerv-specific methods for config management and envelope translation.
// When kit/uxp lands the Adapter interface, HookAdapter will embed it
// directly; until then the base contract is defined inline here.
package internal

// CLIName is a canonical CLI identifier (e.g. "claude", "copilot").
type CLIName string

// CLIInfo holds version and path metadata for a detected CLI.
type CLIInfo struct {
	Name    CLIName `json:"name"`
	Version string  `json:"version"`
	Path    string  `json:"path"`
}

// DetectResult reports whether a CLI is present and its info.
type DetectResult struct {
	Found bool    `json:"found"`
	Info  CLIInfo `json:"info"`
}

// CapabilityMap describes what a CLI adapter supports.
type CapabilityMap struct {
	PreToolUse  bool `json:"pre_tool_use"`
	PostToolUse bool `json:"post_tool_use"`
	PreSession  bool `json:"pre_session"`
	PostSession bool `json:"post_session"`
	Streaming   bool `json:"streaming"`
}

// Adapter is the base contract for CLI adapters.
// Mirrors the planned uxp.Adapter interface from hop.top/kit/uxp.
type Adapter interface {
	// CLI returns the canonical name of the target CLI.
	CLI() CLIName

	// Detect probes whether the CLI is installed and returns its info.
	Detect() (DetectResult, error)

	// Capabilities reports which hook points the CLI supports.
	Capabilities() CapabilityMap
}

// HookAdapter extends Adapter with nerv-specific methods.
type HookAdapter interface {
	Adapter

	// ConfigPaths returns paths to native CLI hook config files
	// that nerv manages (e.g., ~/.claude/settings.json for Claude).
	ConfigPaths() []string

	// MergeConfig merges nerv hook config into the CLI's existing config.
	// Returns the merged config bytes ready to write.
	MergeConfig(existing []byte) ([]byte, error)

	// Translate converts a nerv envelope to CLI-native format.
	Translate(envelope []byte) ([]byte, error)
}
