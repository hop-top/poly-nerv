// Package config loads and merges layered hooks.yaml files.
//
// Layers (lowest to highest priority):
//
//	/etc/nerv/hooks.yaml       (system)
//	~/.config/nerv/hooks.yaml  (user, XDG)
//	.nerv/hooks.yaml           (project)
//	NERV_* env vars            (future)
//
// On parse error the loader logs, skips the bad layer, and continues
// (fail-open per spec/failure-modes.md). The returned error wraps
// [errors.ErrConfig] so callers can inspect the category.
//
// TODO: migrate to kit/config.Load when the full module is wired.
package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"hop.top/kit/go/core/xdg"
	nerverr "hop.top/nerv/internal/errors"
)

// HooksConfig represents the parsed hooks.yaml.
type HooksConfig struct {
	Version  string                    `yaml:"version"`
	Defaults HookDefaults              `yaml:"defaults"`
	Hooks    []HookEntry               `yaml:"hooks"`
	Builtin  map[string]BuiltinUserCfg `yaml:"builtin,omitempty"`
	Bus      BusConfig                 `yaml:"bus,omitempty"`
	Ingest   IngestConfig              `yaml:"ingest,omitempty"`
}

// IngestConfig configures the crtx consume side: bus subscriptions and
// the foreign-session JSONL scanner (see
// docs/architecture/crtx-consumer.md). Loaded only — runtime wiring
// happens in the daemon.
type IngestConfig struct {
	// Enabled toggles consumption. *bool so the layered merge can
	// distinguish an explicit false from key omission; nil defaults
	// to enabled.
	Enabled *bool `yaml:"enabled,omitempty"`
	// ScanRoots lists directories scanned for foreign envelope JSONL.
	// Absolute paths only. When empty the daemon falls back to the
	// well-known sibling-producer default.
	ScanRoots []string `yaml:"scan_roots,omitempty"`
	// ScanIntervalSeconds paces the scanner poll; 0 selects the
	// default (5).
	ScanIntervalSeconds int `yaml:"scan_interval_seconds,omitempty"`
}

// EnabledOrDefault returns the effective Enabled value; omission means
// enabled (consumption is a read-only side channel).
func (c IngestConfig) EnabledOrDefault() bool {
	return c.Enabled == nil || *c.Enabled
}

// BuiltinUserCfg holds per-builtin user configuration. Enabled is the
// safe-default toggle (defaults to false until user opts in). Extra
// captures hook-specific keys without coupling the core config package
// to any individual built-in's schema.
//
// Enabled is *bool so a layered merge can distinguish an explicit value
// (true/false) from key omission. Pseudocode in builtins-registry.md
// uses bool; OQ1's field-level merge requires presence detection,
// matching the *bool pattern already used for HookDefaults.Parallel.
// EnabledOrDefault() returns the effective bool (nil -> false).
type BuiltinUserCfg struct {
	Enabled *bool          `yaml:"enabled,omitempty"`
	Extra   map[string]any `yaml:",inline"`
}

// EnabledOrDefault returns the effective Enabled value, treating a nil
// pointer as false (safe-default; user opts in explicitly).
func (b BuiltinUserCfg) EnabledOrDefault() bool {
	return b.Enabled != nil && *b.Enabled
}

// BusConfig configures the kit/bus runtime adapter, sinks, network
// endpoint, and per-event filters. Loaded only — runtime adapter
// selection happens elsewhere.
type BusConfig struct {
	// Adapter selects the bus implementation: memory | jsonl | sqlite | network.
	// Default: memory.
	Adapter string `yaml:"adapter,omitempty"`
	// Sinks attach output destinations (multi-fanout from the chosen adapter).
	Sinks []BusSink `yaml:"sinks,omitempty"`
	// Network configures the network adapter / sink endpoint.
	Network BusNetwork `yaml:"network,omitempty"`
	// Filters constrain event delivery by name and include/exclude patterns.
	Filters []BusFilter `yaml:"filters,omitempty"`
}

// BusSink is one output destination for bus events.
type BusSink struct {
	// Type names the sink: jsonl | sqlite | stdout | network.
	Type string `yaml:"type"`
	// Path is the file path for jsonl/sqlite sinks. Must be absolute.
	Path string `yaml:"path,omitempty"`
	// Rotate controls log rotation for the jsonl sink.
	Rotate BusRotate `yaml:"rotate,omitempty"`
}

// BusRotate controls jsonl-sink rotation.
type BusRotate struct {
	MaxBytes int64 `yaml:"max_bytes,omitempty"`
	MaxFiles int   `yaml:"max_files,omitempty"`
}

// BusNetwork configures network adapter / sink connectivity.
type BusNetwork struct {
	Endpoint string `yaml:"endpoint,omitempty"`
	Auth     string `yaml:"auth,omitempty"`
}

// BusFilter limits delivery for a single event name.
type BusFilter struct {
	Event   string   `yaml:"event"`
	Include []string `yaml:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty"`
}

// validBusAdapters is the closed set accepted at load time.
var validBusAdapters = map[string]bool{
	"memory":  true,
	"jsonl":   true,
	"sqlite":  true,
	"network": true,
}

// validBusSinkTypes is the closed set accepted at load time.
var validBusSinkTypes = map[string]bool{
	"jsonl":   true,
	"sqlite":  true,
	"stdout":  true,
	"network": true,
}

// defaultBusAdapter is applied when no bus config is set.
const defaultBusAdapter = "memory"

// HookDefaults holds default values applied to all hooks unless
// overridden at the entry level.
type HookDefaults struct {
	Timeout  int    `yaml:"timeout"`  // seconds; 0 means use schema default (10)
	OnError  string `yaml:"on_error"` // "continue" or "abort"
	Parallel *bool  `yaml:"parallel"`
}

// HookEntry describes a single hook definition.
type HookEntry struct {
	Name              string   `yaml:"name"`
	On                any      `yaml:"on"`    // string or []string
	Match             string   `yaml:"match"` // regex
	Run               string   `yaml:"run"`   // path to executable
	RequireCapability string   `yaml:"require_capability"`
	Parallel          *bool    `yaml:"parallel"` // overrides default
	Timeout           *int     `yaml:"timeout"`  // overrides default
	Requires          []string `yaml:"requires"`
	// Disabled short-circuits dispatch for the entry; the daemon emits
	// {decision: skipped, reason: disabled} when honoring it. Loader
	// populates the field but does not enforce it (the daemon
	// pipeline does). Event- and binding-level disables live in
	// .nerv/disabled.yaml — see spec/disabled.schema.json.
	Disabled bool   `yaml:"disabled,omitempty"`
	Source   string `yaml:"-"` // "user" (default, zero-value) | "builtin"; runtime-only
}

// ResolveOn normalises the "on" field to always return []string.
func (h *HookEntry) ResolveOn() []string {
	switch v := h.On.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

// defaultTimeout is the schema-specified default (spec/hooks.schema.json).
const defaultTimeout = 10

// applyDefaults fills zero-value defaults from the schema.
func applyDefaults(cfg *HooksConfig) {
	if cfg.Defaults.Timeout == 0 {
		cfg.Defaults.Timeout = defaultTimeout
	}
	if cfg.Defaults.OnError == "" {
		cfg.Defaults.OnError = "continue"
	}
	if cfg.Bus.Adapter == "" {
		cfg.Bus.Adapter = defaultBusAdapter
	}
}

// validateBus checks the bus section after defaults applied.
// Returns nil for the zero-default case (memory adapter, no sinks).
func validateBus(cfg *HooksConfig, path string) error {
	bus := cfg.Bus

	if !validBusAdapters[bus.Adapter] {
		return nerverr.Wrap(
			nerverr.ErrConfig, "validateBus", path, nil,
			fmt.Sprintf("invalid bus.adapter %q (want memory|jsonl|sqlite|network)", bus.Adapter),
		)
	}

	if bus.Adapter == "network" && bus.Network.Endpoint == "" {
		return nerverr.Wrap(
			nerverr.ErrConfig, "validateBus", path, nil,
			"bus.adapter=network requires bus.network.endpoint",
		)
	}

	for i, s := range bus.Sinks {
		if !validBusSinkTypes[s.Type] {
			return nerverr.Wrap(
				nerverr.ErrConfig, "validateBus", path, nil,
				fmt.Sprintf("invalid bus.sinks[%d].type %q (want jsonl|sqlite|stdout|network)", i, s.Type),
			)
		}
		switch s.Type {
		case "jsonl", "sqlite":
			if s.Path == "" {
				return nerverr.Wrap(
					nerverr.ErrConfig, "validateBus", path, nil,
					fmt.Sprintf("bus.sinks[%d] type=%s requires path", i, s.Type),
				)
			}
			if !filepath.IsAbs(s.Path) {
				return nerverr.Wrap(
					nerverr.ErrConfig, "validateBus", path, nil,
					fmt.Sprintf("bus.sinks[%d].path %q must be absolute", i, s.Path),
				)
			}
		case "network":
			if bus.Network.Endpoint == "" {
				return nerverr.Wrap(
					nerverr.ErrConfig, "validateBus", path, nil,
					fmt.Sprintf("bus.sinks[%d] type=network requires bus.network.endpoint", i),
				)
			}
		}
	}

	return nil
}

// mergeBus performs field-level merge of a layer's BusConfig into merged.
// Later layers override earlier scalars; sinks/filters are appended.
func mergeBus(merged *BusConfig, layer BusConfig) {
	if layer.Adapter != "" {
		merged.Adapter = layer.Adapter
	}
	if layer.Network.Endpoint != "" {
		merged.Network.Endpoint = layer.Network.Endpoint
	}
	if layer.Network.Auth != "" {
		merged.Network.Auth = layer.Network.Auth
	}
	merged.Sinks = append(merged.Sinks, layer.Sinks...)
	merged.Filters = append(merged.Filters, layer.Filters...)
}

// LayerPaths returns the ordered list of hooks.yaml paths to merge.
//
// The user-scope path resolves through kit/xdg.ConfigDir (honors
// XDG_CONFIG_HOME with OS-native fallbacks via adrg/xdg). The project
// layer walks up from cwd to find .nerv/ so a daemon launched outside
// the project root still loads the project hooks.
func LayerPaths() []string {
	paths := []string{"/etc/nerv/hooks.yaml"}

	if dir, err := xdg.ConfigDir("nerv"); err == nil {
		paths = append(paths, filepath.Join(dir, "hooks.yaml"))
	} else if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "nerv", "hooks.yaml"))
	}

	if root, err := FindProjectRoot(); err == nil {
		paths = append(paths, filepath.Join(root, ".nerv", "hooks.yaml"))
	}
	return paths
}

// Load reads hooks.yaml from layered sources and merges them.
//
// Later layers append hooks; the last-seen Version and Defaults win.
// Missing files are silently skipped. Parse errors produce a warning
// log and are collected; the first parse error is returned wrapped as
// [nerverr.ErrConfig] after all layers are attempted.
func Load() (*HooksConfig, error) {
	return loadFromPaths(LayerPaths())
}

// LoadDisabledProject reads .nerv/disabled.yaml from the project
// root, walking up from cwd to find .nerv/ the same way the CLI
// disable surface (disable.OpenProject) does.
//
// Without the walk-up the daemon was cwd-relative: a `nervd` started
// from anywhere other than the project root (e.g. /tmp, a system
// service launcher's cwd) silently loaded no sidecar even though the
// CLI happily wrote to <project>/.nerv/disabled.yaml. With the
// walk-up the read path matches the write path.
//
// Absent file (or no project root found) → zero-value sidecar;
// parse errors are logged and swallowed so dispatch keeps running
// on a malformed sidecar (fail-open per nerv contract).
func LoadDisabledProject() *Disabled {
	root, err := FindProjectRoot()
	if err != nil {
		// No .nerv/ on the path upward — equivalent to "no sidecar".
		// Don't log: a daemon running outside any nerv project is a
		// normal stateless-mode invocation, not a misconfiguration.
		return &Disabled{Version: DisabledSidecarVersion}
	}
	d, err := LoadDisabled(filepath.Join(root, ".nerv", "disabled.yaml"))
	if err != nil {
		log.Printf("config: skip %s/.nerv/disabled.yaml: %v", root, err)
		return &Disabled{Version: DisabledSidecarVersion}
	}
	return d
}

// loadFromPaths is the internal loader; exported indirectly via Load.
func loadFromPaths(paths []string) (*HooksConfig, error) {
	merged := &HooksConfig{}
	var firstErr error

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			// Missing file is not an error.
			if os.IsNotExist(err) {
				continue
			}
			log.Printf("config: skip %s: %v", p, err)
			if firstErr == nil {
				firstErr = nerverr.Wrap(
					nerverr.ErrConfig, "Load", p, err,
					fmt.Sprintf("could not read %s", p),
				)
			}
			continue
		}

		var layer HooksConfig
		if err := yaml.Unmarshal(data, &layer); err != nil {
			log.Printf("config: skip %s: bad yaml: %v", p, err)
			if firstErr == nil {
				firstErr = nerverr.Wrap(
					nerverr.ErrConfig, "Load", p, err,
					fmt.Sprintf("invalid YAML in %s", p),
				)
			}
			continue
		}

		// Merge: later layer wins for scalar fields.
		if layer.Version != "" {
			merged.Version = layer.Version
		}
		if layer.Defaults.Timeout != 0 {
			merged.Defaults.Timeout = layer.Defaults.Timeout
		}
		if layer.Defaults.OnError != "" {
			merged.Defaults.OnError = layer.Defaults.OnError
		}
		if layer.Defaults.Parallel != nil {
			merged.Defaults.Parallel = layer.Defaults.Parallel
		}

		// Hooks are appended; later layers add, not replace.
		merged.Hooks = append(merged.Hooks, layer.Hooks...)

		// Built-in configs merge field-by-field across layers (OQ1).
		// Project layer setting only Extra["drift_keywords"] does NOT
		// clobber Enabled from the user layer; Extra also merges per-key.
		mergeBuiltin(&merged.Builtin, layer.Builtin)

		// Bus merges field-by-field; sinks/filters append.
		mergeBus(&merged.Bus, layer.Bus)

		// Ingest merges field-by-field; scan roots append.
		mergeIngest(&merged.Ingest, layer.Ingest)
	}

	applyDefaults(merged)

	if err := validateBus(merged, ""); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	if err := validateIngest(merged, ""); err != nil {
		if firstErr == nil {
			firstErr = err
		}
	}

	return merged, firstErr
}

// mergeIngest performs field-level merge of a layer's IngestConfig.
// Enabled is overwritten only when the layer set it explicitly;
// scan roots append across layers; a non-zero interval wins.
func mergeIngest(merged *IngestConfig, layer IngestConfig) {
	if layer.Enabled != nil {
		merged.Enabled = layer.Enabled
	}
	merged.ScanRoots = append(merged.ScanRoots, layer.ScanRoots...)
	if layer.ScanIntervalSeconds != 0 {
		merged.ScanIntervalSeconds = layer.ScanIntervalSeconds
	}
}

// validateIngest checks the ingest section after merge.
func validateIngest(cfg *HooksConfig, path string) error {
	for i, root := range cfg.Ingest.ScanRoots {
		if !filepath.IsAbs(root) {
			return nerverr.Wrap(
				nerverr.ErrConfig, "validateIngest", path, nil,
				fmt.Sprintf("ingest.scan_roots[%d] %q must be absolute", i, root),
			)
		}
	}
	if cfg.Ingest.ScanIntervalSeconds < 0 {
		return nerverr.Wrap(
			nerverr.ErrConfig, "validateIngest", path, nil,
			fmt.Sprintf("ingest.scan_interval_seconds %d must be >= 0", cfg.Ingest.ScanIntervalSeconds),
		)
	}
	return nil
}

// mergeBuiltin folds layer's Builtin map into dst with field-level
// granularity (OQ1). For each name present in layer:
//   - Enabled is overwritten only when the layer set it explicitly
//     (non-nil); a layer that omits the key preserves the prior value.
//   - Extra is merged per-key: later layer wins on collisions; keys
//     unique to dst are preserved.
//
// dst is allocated lazily so a nil top-level Builtin map only becomes
// non-nil if at least one layer contributes an entry. This mirrors the
// HookDefaults merge pattern (Parallel *bool).
func mergeBuiltin(dst *map[string]BuiltinUserCfg, layer map[string]BuiltinUserCfg) {
	if len(layer) == 0 {
		return
	}
	if *dst == nil {
		*dst = make(map[string]BuiltinUserCfg, len(layer))
	}
	for name, in := range layer {
		cur := (*dst)[name]
		if in.Enabled != nil {
			cur.Enabled = in.Enabled
		}
		if len(in.Extra) > 0 {
			if cur.Extra == nil {
				cur.Extra = make(map[string]any, len(in.Extra))
			}
			for k, v := range in.Extra {
				cur.Extra[k] = v
			}
		}
		(*dst)[name] = cur
	}
}
