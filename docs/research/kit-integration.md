# hop-top/kit — nerv Integration Map

> Which nerv concerns delegate to which kit package. Keeps nerv thin.
> Living doc; update when kit ships new packages or changes APIs.

Last verified: 2026-04-11. Kit README:
<https://github.com/hop-top/kit/blob/main/README.md>

## Kit packages nerv uses

| Package       | Purpose                                            | nerv usage                                                      |
|---------------|----------------------------------------------------|----------------------------------------------------------------|
| `kit/cli`     | fang+cobra+viper root command factory              | `nerv` binary root + all subcommands; free `--format`/`--quiet`/`--no-color`/`--version` |
| `kit/config`  | Layered config loader (system→user→project→env)    | `hooks.yaml` loader (global + project merge); capability maps  |
| `kit/log`     | Viper-configured `charm.land/log/v2` wrapper       | Daemon + adapter logging (structured, level-aware)             |
| `kit/xdg`     | XDG path resolution (config/data/cache/state)      | All paths: socket, event log, pidfile, state                   |
| `kit/output`  | table/json/yaml renderer (owns `--format`)         | `nerv doctor`, `tail`, `status`, `lint` outputs                 |
| `kit/sqlstore`| Generic SQLite kv store with TTL                    | Event log (replaces JSONL); TTL-based retention                |
| `kit/tui`     | Themed bubbletea/v2 components (spinner/confirm/etc)| `nerv install` wizard, progress bars during adapter install    |
| `kit/upgrade` | Self-upgrade check/download/replace                | `nerv upgrade` command                                          |
| `kit/markdown`| Glamour v2 terminal markdown renderer              | `nerv doctor` rich reports; help text rendering                 |
| `kit/uxp`     | Shared AI CLI detection, adapter interface, doctor framework, project key derivation | Adapter interface embedding, CLI detection for install-status, doctor base framework, store path resolution |

## kit/uxp — AI CLI detection + adapter interface

`hop.top/kit/uxp@v0.2.0-alpha.1` provides shared infrastructure for
any tool that needs to detect, query, or adapt to AI coding CLIs.

### Core types

- **`CLIInfo`** — canonical metadata per AI CLI: name, binary names,
  store roots, config patterns, project key strategy
- **`CLIName`** (string type) — constants for 13 CLIs: `claude`,
  `gemini`, `codex`, `opencode`, `copilot`, `cursor-agent`, `amp`,
  `kimi`, `qwen`, `vibe`, `antigravity`, `tabnine`, `windsurf`
- **`CLIRegistry`** — static map `CLIName→CLIInfo`; methods:
  `All()`, `Get()`, `Names()`

### Adapter interface

- **`Adapter`** — `CLI()`, `Detect()`, `Capabilities()`
- **`DetectResult`** — `Installed`, `Version`, `BinaryPath`,
  `ConfigPaths`, `Errors`
- **`CapabilityMap`** — `Supports()`, `Coverage()`

### Doctor framework

- **`Check`** struct — `Name`, `Status`, `Message`, `Detail`
- **`CheckStatus`** enum — `OK`, `Warn`, `Fail`, `Skip`
- **`Doctor`** — `Run()`, `Add()`

### Project key derivation

- **`ProjectKeyStrategy`** enum — `SlashToDash`, `SHA1`, `SHA256`,
  `MD5`, `BasenameAlias`, `Embedded`, `None`
- **`DeriveKey(cwd, strategy)`** — project key derivation
- **`ResolveStorePath(cli)`** — store root resolution with
  tilde/env expansion

### Detection

- **`Detect(cli)`** — CLI installation detection via
  `exec.LookPath` + version parse

### nerv usage

- Adapter interface: nerv adapters embed `uxp.Adapter` instead of
  defining their own
- `nerv install <cli>`: calls `uxp.Detect()` to check CLI presence
  and version before writing adapter config
- `nerv doctor`: builds on `uxp.Doctor` framework for health checks
- `nerv status`: uses `uxp.CLIRegistry` for known-CLI enumeration
- Store path resolution via `uxp.ResolveStorePath()` for locating
  CLI-specific hook configs

## Kit packages nerv may skip (v1)

| Package       | Reason                                             |
|---------------|----------------------------------------------------|
| `kit/llm`     | nerv doesn't call LLMs; handlers do if they want    |
| `kit/templates`| Install-time config templating — evaluate in Phase 3 |

## Cross-language kit

nerv also uses kit's TS and Python ports for the SDKs:

| Kit package          | nerv SDK using it           |
|----------------------|----------------------------|
| `@hop/kit-ts/cli`    | `sdk/ts/` (commander wrapper baseline for any CLI hook helper) |
| `@hop/kit-ts/output` | SDK output formatting      |
| `hop_kit.cli`        | `sdk/py/` (Typer baseline) |
| `hop_kit.output`     | SDK output formatting      |

## What nerv builds from scratch vs delegates

### Delegates to kit

- CLI bootstrap (`kit/cli`) — nerv never touches fang/cobra/viper
  directly
- Config loading (`kit/config`) — layered `hooks.yaml` merge
- Structured logging (`kit/log`) — every daemon/adapter log line
- XDG paths (`kit/xdg`) — socket, state, cache, logs
- Output formatting (`kit/output`) — `--format` flag handled for free
- Event persistence (`kit/sqlstore`) — events queryable by event name,
  cli, session, timestamp; TTL retention policy
- Self-update (`kit/upgrade`)
- Interactive install wizard (`kit/tui`)
- CLI detection (`kit/uxp`) — `Detect()` for install-status checks
- Adapter interface (`kit/uxp`) — `Adapter` interface embedding
- Doctor checks (`kit/uxp`) — `Doctor` base framework for health
  checks
- Project key derivation (`kit/uxp`) — `DeriveKey()` +
  `ResolveStorePath()` for CLI store paths

### nerv-specific (can't delegate)

- Unix socket server + length-prefixed JSON framing (not in kit; too
  nerv-specific)
- Envelope JSON schema validation (kit has no jsonschema helper;
  pick `santhosh-tekuri/jsonschema` directly)
- Matcher engine (regex/tool-name matching against hooks.yaml rules)
- Decision merger (block > rewrite > warn > allow semantics)
- Handler runners (exec / http / grpc dispatch; specific to nerv
  contract)
- Per-CLI adapter install logic (writes native hook config files;
  uses `uxp.Detect()` + `uxp.ResolveStorePath()` but install
  logic itself is nerv-specific)
- Synthesis helpers (shell regex decoder, fswatch bridge, binary
  wrapper, MCP proxy)

Note: adapter interface definition moved to `kit/uxp` — nerv adapters
embed `uxp.Adapter` rather than defining their own interface.

## CLI contract nerv inherits free

From `kit/cli` every nerv command gets:

- `--format {table,json,yaml}` — rendered via `kit/output.Render()`
- `--quiet` — suppress non-essential output
- `--no-color` — disable ANSI
- `-v`/`--version` — root-only, handled by fang
- `-h`/`--help` — at any level, styled by fang
- Accent color (pick one for nerv, e.g., `#00BFA5`)
- Structured error output to stderr, non-zero exit, no stack traces

## Paths (via `kit/xdg`)

```pseudocode
xdg.ConfigDir("nerv")    -> ~/.config/nerv/       # global hooks.yaml, settings
xdg.DataDir("nerv")      -> ~/.local/share/nerv/  # installed adapters/packs
xdg.CacheDir("nerv")     -> ~/.cache/nerv/        # schema cache, compiled matchers
xdg.StateDir("nerv")     -> ~/.local/state/nerv/  # pidfile, event db, logs
```

Runtime socket: `$XDG_RUNTIME_DIR/nerv.sock` (resolved via kit/xdg or
stdlib fallback on darwin).

## Config layering (via `kit/config`)

```pseudocode
config.Load(&cfg, config.Options{
  Name: "nerv",
  Layers: []string{
    "/etc/nerv/config.yaml",              # system (optional)
    xdg.ConfigDir("nerv") + "/config.yaml", # user global
    ".nerv/config.yaml",                    # project
    // env vars prefixed NERV_*
  },
})
```

Same pattern for `hooks.yaml` discovery — nerv's hook loader consumes
`kit/config` layers and merges hook lists.

## Logging (via `kit/log`)

```pseudocode
// kit/cli.New() wires log automatically from viper flags
log := log.New(root.Viper)
log.Info("envelope received", "event", env.Event, "cli", env.Cli.Name)
log.Error("handler failed", "hook", hook.Name, "err", err)
```

Log level controlled by `--log-level` flag (free from kit). No custom
logger needed.

## Event persistence (via `kit/sqlstore`)

Replace the originally-planned JSONL log with a SQLite store:

```pseudocode
store := sqlstore.Open(xdg.StateDir("nerv") + "/events.db", sqlstore.Options{
  TTL: 30 * 24 * time.Hour,
})

// Append envelope + decision as key with indexed metadata
store.Put(event_id, envelope_plus_decision_json, sqlstore.Meta{
  "event": env.Event,
  "cli":   env.Cli.Name,
  "session": env.Session.Id,
  "ts":    env.Timestamp,
})

// Query for nerv tail / replay / doctor
store.Query(sqlstore.Filter{Event: "tool.pre", Since: yesterday})
```

Benefits over JSONL:
- Fast filtered tails (`nerv tail --event=tool.pre --cli=codex`)
- Reliable replay by event_id
- TTL retention without manual rotation
- Smaller on-disk footprint via SQLite pages

## TUI wizard (via `kit/tui`)

```pseudocode
// nerv install codex
spinner := tui.NewSpinner("Detecting Codex CLI version...")
// ...detect...
spinner.Success("Found codex 0.42.0")

if !tui.Confirm("Install Layers 1+2 (basic)? Full install needs MCP proxy.") {
  return
}

prog := tui.NewProgress(4, "Installing adapter")
prog.Step("Writing ~/.codex/hooks.json")
prog.Step("Setting codex_hooks=true")
prog.Step("Installing fswatch daemon")
prog.Step("Verifying socket reachable")
prog.Done("Codex adapter installed (3 native + 11 synthesized events)")
```

## Open questions

1. Does `kit/config` support hot reload via fsnotify, or does nerv need
   to layer that itself?
2. Does `kit/sqlstore` support compound indexes for fast
   `event+cli+session` filtering, or only single-key lookups?
3. Is `kit/upgrade` compatible with the version matrix concept (where
   nerv itself has versioning + per-adapter versioning)?
4. Does `@hop/kit-ts/cli` expose Node-side stdin/stdout JSON helpers
   useful for the nerv SDK, or is it commander-only?
5. `kit/templates` — enough to generate per-CLI install configs
   (e.g., `~/.claude/settings.json` stanzas), or should nerv use stdlib
   `text/template`?

## Integration checklist (for Phase 2 bootstrap task)

- [ ] `go get hop.top/kit@latest`
- [ ] `cmd/nerv/main.go` uses `cli.New(cli.Config{Name:"nerv", Accent:"#00BFA5"})`
- [ ] All subcommand files use `root.Viper` for flags
- [ ] Replace any ad-hoc `os.UserConfigDir()` with `xdg.ConfigDir("nerv")`
- [ ] Replace any `fmt.Printf` with `log.Info/Error` from `kit/log`
- [ ] Replace any hand-rolled renderers with `output.Render(w, format, v)`
- [ ] Delete any copy-pasted CLI contract code (fang handles it)
- [ ] Use `kit/sqlstore` for event log (not JSONL)
- [ ] Use `kit/tui` for `install` wizard
- [ ] Use `kit/upgrade` for `nerv upgrade`
- [ ] Use `kit/uxp` `Adapter` interface for all CLI adapters
- [ ] Use `kit/uxp` `Detect()` in `nerv install` pre-flight
- [ ] Use `kit/uxp` `Doctor` framework in `nerv doctor`
- [ ] Use `kit/uxp` `ResolveStorePath()` for CLI config paths
- [ ] Use `kit/uxp` `CLIRegistry` for known-CLI enumeration
