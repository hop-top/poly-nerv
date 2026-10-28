---
id: builtins-registry
title: "Built-in hook registry pattern"
status: accepted
date: 2026-04-26
author: jadb
refs:
  - internal/config/loader.go
  - internal/config/cache.go
  - internal/dispatch/run.go
  - internal/hookpkg/install.go
  - adapters/registry.go
---

# Built-in hook registry pattern

Diagram: [dispatch-flow-v1.mmd](builtins-registry/dispatch-flow-v1.mmd)

---

## Context

nerv ships zero built-in hooks today. All hooks are user-installed
via `nerv install` (or manually authored in `hooks.yaml`). The
open-questions detector is the first
built-in candidate. It needs:

- presence in binary (no external handler script to install)
- lifecycle tied to the daemon (loaded at startup, not on-demand)
- per-user enable/disable toggle (default off until stable)
- appearance in `nerv hook list` as `builtin:open-questions`
- same dispatch guarantees (fail-open, timeout, merge) as user hooks

This ADR specifies the registry pattern that satisfies all five.

---

## 1. Registration mechanism

**Decision: compile-time slice in `internal/hookpkg/builtins/registry.go`.**

Rejected: `init()` registration (package side-effect ordering is fragile;
builds without the package silently drop the hook; hard to enumerate for
`hook list`).

Chosen: explicit `var All = []BuiltinDef{ ... }` in a single file.
Each new built-in is a one-line append. Mirrors `adapters/registry.go`
(`Registry()` returns a compile-time map of all adapters). Order in
`All` is execution order when multiple built-ins subscribe to the same
event — deterministic, easy to reason about.

---

## 2. Lifecycle

Built-ins are injected into `CachedConfig.ByEvent` during
`config.Compile()`, which runs at daemon startup and on SIGHUP reload.
No separate init step — the same cache-compile path that indexes user
hooks also injects built-ins.

`builtins.Inject(cfg, byEvent)` is called at the end of `Compile()`,
after all user hooks are indexed. Built-ins always appear AFTER user
hooks in the event bucket (user hooks run first; built-ins act as
trailing observers).

`nerv hook list` reads `CachedConfig.ByEvent` and renders entries tagged
`Source: "builtin"` with a `builtin:` prefix. No separate list path
needed.

---

## 3. Per-builtin enable flag

New top-level key in `hooks.yaml`:

```yaml
builtin:
  open-questions:
    enabled: false        # safe default; user opts in
    # hook-specific keys follow (hook owns schema below this)
```

`HooksConfig.Builtin` is `map[string]BuiltinUserCfg`. The core `enabled`
flag is a typed field; hook-specific config lives in an inline map
(gopkg.in/yaml.v3 `yaml:",inline"` on `map[string]any`) so the core
config package never needs to know about open-questions-specific keys.

Per the layered-config design (`internal/config/loader.go`), the user
layer (`~/.config/nerv/hooks.yaml`) can enable a built-in globally;
a project layer (`.nerv/hooks.yaml`) can override. Later layer wins for
scalar fields, including `enabled` (see OQ1 below for merge granularity).

---

## 4. Dispatch path

**Decision: same path as user hooks.**

Built-ins produce synthetic `HookEntry` records with `Source: "builtin"`.
In `dispatch/run.go`, `RunHooks` checks `h.Source` before forking:

```
if h.Source == "builtin" {
    d = r.runBuiltin(h, env, cfg)
} else {
    d = r.runOne(h, env, defaults)   // existing fork path, unchanged
}
```

`runBuiltin` does:

1. Strip `builtin:` prefix → lookup `builtins.ByName(name)`.
2. Check `cfg.Builtin[name].Enabled` → return allow if false.
3. Call `def.Func(env, runtimeCfg)` wrapped in `recoverDecision()`
   (panic → fail-open; same guarantee as exec-timeout for user hooks).
4. Return decision.

No separate fast-path. Built-ins benefit from the same fail-open machinery
(`failopen.go`), merger, and event log as user hooks. A separate fast-path
would bypass those guarantees for no measurable gain.

---

## 5. Versioning + migration

Each `BuiltinDef` carries `SchemaVersion int`. When a built-in's config
schema changes (e.g., a key is renamed or a new required field is added),
`SchemaVersion` is incremented.

`CachedConfig` stores `BuiltinVersions map[string]int` (name → version
at compile time). On daemon startup, after loading user config:

1. Compare `cfg.BuiltinVersions[name]` (last-seen) vs
   `def.SchemaVersion` (current binary).
2. If current > last-seen AND `def.Migrator != nil`:
   call `def.Migrator.Migrate(fromVersion, stateDir)`.
3. Emit a log warning visible via `nerv daemon status` listing migrated
   built-ins and their old/new versions.
4. Update `cfg.BuiltinVersions[name]` to current.

State migration only touches the built-in's own state dir
(`~/.nerv/state/<name>/`). Config schema migrations (YAML key renames)
emit a deprecation warning to stderr but do NOT break loading — old keys
are retained via `Extra map[string]any` until the user updates their
config. This follows the fail-open principle.

Phase 1 built-ins (open-questions v1) set `SchemaVersion: 1` and
`Migrator: nil`.

---

## 6. Disabling semantics

When `Builtin["open-questions"].Enabled == false`:

- The synthetic `HookEntry` IS still injected into `ByEvent` by
  `builtins.Inject`. The built-in appears in `nerv hook list` with
  status `disabled`.
- At dispatch time, `runBuiltin` short-circuits to `allow` before
  calling `Func`. Zero cost to the dispatch hot path.
- No config rewrite or removal needed — disable/enable is a live
  config toggle, effective on next SIGHUP or daemon restart.
- The built-in's state dir and inbox dir are left intact when disabled;
  no cleanup. Only `nerv builtin purge <name>` (future command) removes
  them.

---

## 7. Open-questions hook — first-consumer walkthrough

### Registration

```go
// internal/hookpkg/builtins/registry.go
var All = []BuiltinDef{
    openquestions.Def, // package: internal/hookpkg/builtins/openquestions
}
```

### Config (user's ~/.config/nerv/hooks.yaml)

```yaml
builtin:
  open-questions:
    enabled: true
    llm:
      enabled: true
      provider: nerv-primary
      budget_tokens: 2000
    drift_keywords:
      - "moving on"
      - "let's switch"
      - "different topic"
      - "next:"
    state_ttl_hours: 168
    inbox_path: ~/.nerv/inbox/<agent_id>/open-questions/
```

### BuiltinDef

```go
// internal/hookpkg/builtins/openquestions/builtin.go
var Def = builtins.BuiltinDef{
    Name:          "open-questions",
    SchemaVersion: 1,
    Events: []string{
        "UserPromptSubmit",
        "PreToolUse",   // drift: tlc task claim, Task tool dispatch
        "CwdChanged",   // drift: repo switch
        "Stop",         // per-turn finalization
        "SessionEnd",   // force-flush
        "PreCompact",   // force-flush before context compression
    },
    Func:     Handle,
    Migrator: nil, // no migration for v1
}
```

### Runtime config decode

```go
// internal/hookpkg/builtins/openquestions/config.go
type Config struct {
    LLM            LLMConfig `yaml:"llm"`
    DriftKeywords  []string  `yaml:"drift_keywords"`
    StateTTLHours  int       `yaml:"state_ttl_hours"`
    InboxPath      string    `yaml:"inbox_path"`
}

func DecodeConfig(raw map[string]any) (Config, error) {
    // marshal raw → YAML bytes → unmarshal into Config
    // (avoids go-mapstructure dep; uses existing gopkg.in/yaml.v3)
}
```

### `nerv hook list` output (example)

```
NAME                           SOURCE    EVENTS              STATUS
builtin:open-questions         builtin   6 events            enabled
my-custom-hook                 user      PreToolUse          active
```

---

## Go types — schema additions

These types are the contract for phase 2 implementation.
Exact code: use pseudocode below as spec; do not copy verbatim.

### `internal/config/loader.go` additions

```go
// BuiltinUserCfg holds per-builtin user configuration.
// enabled: user toggle; defaults false (safe off).
// Inline map captures hook-specific keys without core package coupling.
type BuiltinUserCfg struct {
    Enabled bool           `yaml:"enabled"`
    Extra   map[string]any `yaml:",inline"`
}

// HooksConfig — add Builtin field
type HooksConfig struct {
    Version  string                    `yaml:"version"`
    Defaults HookDefaults              `yaml:"defaults"`
    Hooks    []HookEntry               `yaml:"hooks"`
    Builtin  map[string]BuiltinUserCfg `yaml:"builtin,omitempty"` // NEW
}

// HookEntry — add Source field (not serialized; set at runtime)
type HookEntry struct {
    Name              string   `yaml:"name"`
    On                any      `yaml:"on"`
    Match             string   `yaml:"match"`
    Run               string   `yaml:"run"`
    RequireCapability string   `yaml:"require_capability"`
    Parallel          *bool    `yaml:"parallel"`
    Timeout           *int     `yaml:"timeout"`
    Requires          []string `yaml:"requires"`
    Source            string   `yaml:"-"` // NEW: "user" (default) | "builtin"
}
```

### `internal/config/cache.go` addition

```go
// CachedConfig — add BuiltinVersions
type CachedConfig struct {
    Config          *HooksConfig
    ByEvent         map[string][]HookEntry
    Version         string
    SourceMod       time.Time
    BuiltinVersions map[string]int // NEW: name → SchemaVersion at compile time
}
```

### `internal/hookpkg/builtins/registry.go` (new file)

```go
package builtins

import (
    "hop.top/nerv/internal/config"
    "hop.top/nerv/internal/envelope"
    "hop.top/nerv/internal/merger"
)

// BuiltinFunc is the in-process handler. Same contract as exec handler:
// receives envelope, returns decision. Panics are caught by caller.
type BuiltinFunc func(
    env *envelope.Envelope,
    cfg BuiltinRuntimeCfg,
) (merger.Decision, error)

// BuiltinRuntimeCfg carries per-call deps for a built-in handler.
type BuiltinRuntimeCfg struct {
    UserCfg  map[string]any // raw map under hooks.builtin.<name>
    StateDir string         // $HOME/.nerv/state/<name>/
    InboxDir string         // resolved inbox_path from cfg or default
}

// BuiltinDef declares a compiled-in hook.
type BuiltinDef struct {
    Name          string
    SchemaVersion int      // bump on breaking config/state schema change
    Events        []string // events to subscribe
    Func          BuiltinFunc
    Migrator      Migrator // nil = no migration
}

// Migrator upgrades stored state when SchemaVersion bumps.
type Migrator interface {
    Migrate(fromVersion int, stateDir string) error
}

// All is the compile-time registry. Add new entries here.
var All = []BuiltinDef{}

// ByName returns the BuiltinDef for name, if registered.
func ByName(name string) (BuiltinDef, bool) {
    for _, d := range All {
        if d.Name == name {
            return d, true
        }
    }
    return BuiltinDef{}, false
}

// Inject appends synthetic HookEntry records for all built-ins into
// byEvent. Called from config.Compile() after user hooks are indexed.
// Built-ins appear after user hooks in each event bucket.
func Inject(cfg *config.HooksConfig, byEvent map[string][]config.HookEntry) {
    for _, def := range All {
        for _, ev := range def.Events {
            byEvent[ev] = append(byEvent[ev], config.HookEntry{
                Name:   "builtin:" + def.Name,
                On:     ev,
                Source: "builtin",
                // Run intentionally empty; dispatch routes via Source
            })
        }
    }
}
```

### `internal/dispatch/run.go` routing addition

```go
// In RunHooks, before existing runOne call — pseudocode:
if h.Source == "builtin" {
    decisions = append(decisions, r.runBuiltin(h, env, cfg.Config))
    continue
}

// runBuiltin dispatches to the in-process BuiltinDef.Func.
// Fail-open on: disabled, unknown name, panic, error.
func (r *Runner) runBuiltin(
    h config.HookEntry,
    env *envelope.Envelope,
    cfg *config.HooksConfig,
) merger.Decision {
    name := strings.TrimPrefix(h.Name, "builtin:")
    def, ok := builtins.ByName(name)
    if !ok {
        log.Printf("dispatch: unknown builtin %q (fail-open)", name)
        return allowDecision("fail-open: unknown builtin")
    }
    if !cfg.Builtin[name].Enabled {
        return merger.Decision{Action: merger.Allow} // disabled, silent
    }
    rCfg := resolveBuiltinRuntimeCfg(name, cfg)
    decision, err := callBuiltinSafe(def, env, rCfg)
    if err != nil {
        log.Printf("dispatch: builtin %q error (fail-open): %v", name, err)
        return allowDecision("fail-open: builtin error")
    }
    return decision
}

// callBuiltinSafe calls def.Func recovering from panics.
func callBuiltinSafe(
    def builtins.BuiltinDef,
    env *envelope.Envelope,
    rCfg builtins.BuiltinRuntimeCfg,
) (d merger.Decision, err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("panic: %v", r)
        }
    }()
    return def.Func(env, rCfg)
}
```

---

## Trade-offs accepted

| Concern | Decision |
|---|---|
| Built-in code in binary increases binary size | Acceptable; built-ins are small; no runtime dep |
| Disabling doesn't remove from ByEvent bucket | Intentional; visibility in `hook list` is desirable |
| `Source` field not in YAML (yaml:"-") | Built-ins are never serialized to yaml; correct |
| init() rejected | Ordering fragility and silent omission risk outweigh brevity |
| Same dispatch path (not fast-path) | Fail-open parity with user hooks is more valuable |

---

## Phase 2 implementation tasks

1. **infra/config** — add `BuiltinUserCfg`, `Builtin` field to
   `HooksConfig`, `Source` to `HookEntry`, `BuiltinVersions` to
   `CachedConfig`; update `applyDefaults`; update loader merge logic
   for `Builtin` map.
2. **infra/builtins-pkg** — create
   `internal/hookpkg/builtins/registry.go` with types + `Inject` +
   `ByName`; no built-ins registered yet.
3. **infra/compile** — call `builtins.Inject(cfg, byEvent)` in
   `config.Compile()`; add `BuiltinVersions` population.
4. **infra/dispatch** — add `runBuiltin` + `callBuiltinSafe` +
   `resolveBuiltinRuntimeCfg` to `dispatch/run.go`.
5. **infra/hook-list** — add `nerv hook list` command reading
   `CachedConfig` and rendering `Source` column + disabled/enabled
   status.
6. **infra/migration** — add `Migrator` interface + migration
   call-site in daemon startup sequence.
7. **open-questions/builtin** — create
   `internal/hookpkg/builtins/openquestions/` package: `Config`,
   `Def`, `Handle` stub; register in `builtins.All`. (Separate task
   spec covers full detector implementation.)

Each task is independent after task 1 (config changes); tasks 2-4 can
proceed in parallel once the config types are merged.

---

## Resolved decisions (2026-04-26 founder review)

OQ1–OQ3 resolved during the 2026-04-26 design pass.

- **OQ1 (Builtin map merge strategy) — RESOLVED: field-level merge.**
  Each `BuiltinUserCfg` entry merges field-by-field across layers
  (system → user → project). Project layer setting `drift_keywords`
  alone does NOT clobber `enabled` from the user layer. Same merge
  logic applied to `HookDefaults` today. Implementation note: the
  `Extra map[string]any` field also merges per-key.

- **OQ2 (agent_id source) — RESOLVED: aps lookup at daemon startup.**
  nervd queries aps for the active profile during `config.Compile()`
  and stores the resolved id in a new `CachedConfig.ActiveProfile`
  field. `resolveBuiltinRuntimeCfg` reads from that field; tokens
  like `<agent_id>` in `inbox_path` are expanded against it. NO
  envelope schema change. NO env var. Phase 2 task: add
  `aps.LookupActiveProfile()` helper if it doesn't exist; wire into
  daemon startup.

- **OQ3 (`nerv hook list` vs `nerv hooks`) — RESOLVED: subcommand
  group.** New `nerv hook` group (mirrors `nerv daemon` precedent at
  `cmd/nerv/cmd_daemon.go:61-74`). Initial subcommand: `nerv hook
  list`. Future absorbs `enable/disable/purge`. Phase 2 task 5
  filed under this surface.

### Cross-component impact (newly added)

- **kit/bus dependency:** This ADR's `BuiltinRuntimeCfg` gets a new
  field `Bus bus.Bus` so built-ins can publish events. Built-ins
  that emit events (e.g. open-questions hook publishes
  `inbox_message_*`) depend on kit/bus adoption
  (`kit-bus-adoption.md`) shipping phase 1 first. Pure non-emitting built-ins (none yet) wouldn't
  need this.

- **Sibling ADR:** `kit-bus-adoption.md` —
  prerequisite for any built-in that publishes bus events.
