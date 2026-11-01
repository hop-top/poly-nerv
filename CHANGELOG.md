# Changelog — nerv

All notable changes to nerv land here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows
[SemVer](https://semver.org/) with `alpha.N` prerelease channel during 0.x.

This file is owned by release-please from `0.1.0-alpha.0` onward — the
`## Unreleased` block below documents the in-flight changes that will land
in the first tagged release.

## Unreleased

### OpenCode v2 bridge and install

- `nerv install add opencode` now works: the generic installer supports
  multi-file adapters (`MultiFileAdapter`), journaling, backing up,
  verifying, rolling back and uninstalling every file, and the registry
  gives the opencode adapter the `nervd` command (PATH-resolved, like the
  gemini and codex adapters). Before, only `opencode.json` was a managed
  path and the empty nervd path made the command fail; the bridge plugin
  was only reachable through the adapter's own `Install`.
- OpenCode plugin API v2 bridge (`Plugin.define` + `setup(ctx)`), selected
  by `opencode --version`: installs `.opencode/nerv-bridge/index.ts` and a
  `plugins` entry in `opencode.json`; switching OpenCode generations
  replaces the other bridge in place. Same `nervd --adapter opencode`
  wire shape: tool before (block) and after (context), session context on
  the system prompt, bus events via `ctx.event.subscribe()`.
- Pinned OpenCode 2.2.5 (GitHub release; not on npm) next to 1.19.3
  (`versions.yaml`, `cli_version_range`, `opencode-v2` compose service).

### Goose hooks

- `nervd --adapter goose` route: decodes Goose plugin-hook payloads
  (`event` field) and answers block on PreToolUse and Stop as
  `{"decision":"block","reason":...}` on stdout; allow, warn, rewrite are
  empty stdout, context is dropped (no channel).
- Goose install now writes `~/.agents/plugins/nerv/hooks/hooks.json`
  (command `nervd --adapter goose`) instead of an MCP extension running
  the nonexistent `nerv dispatch goose`; legacy commands in that file are
  upgraded in place, idempotent. Remove any old `nerv-bridge` entry from
  `~/.config/goose/config.yaml` by hand.
- Pinned goose 1.55.0 (`versions.yaml`, `cli_version_range >=1.55.0`).
- Goose capability claims corrected to the 8 delivered plugin hooks
  (8/26); undelivered SSE and shell-synthesized events listed unsupported.

### Removed

- Vibe (Mistral) and Aider adapters, with their `nervd --adapter vibe` /
  `aider` routes, argv input path, install wiring, CI containers, and
  docs. Neither reached parity; unsupported harnesses.

### Handler decision `context`

- **Optional `context` string on every decision** — model-facing text,
  valid with any action; exit codes unchanged. Merged in hook order
  (newline-separated), capped at 8 KiB with a truncation marker,
  dropped when the merged action is `block`.
- Claude adapter emits it as `hookSpecificOutput.additionalContext`
  on SessionStart, SubagentStart, UserPromptSubmit, PreToolUse,
  PostToolUse, PostToolUseFailure, Stop, SubagentStop (also alongside
  a PreToolUse rewrite); other events drop it.
- Gemini adapter: new `FromEventDecision(d, geminiEvent)` emits
  `hookSpecificOutput.additionalContext` on SessionStart, BeforeAgent,
  AfterTool.
- Codex adapter: new `FromEventDecision(d, codexEvent)` emits
  `hookSpecificOutput.additionalContext` on SessionStart,
  UserPromptSubmit, PreToolUse, PostToolUse (Stop/compaction outputs
  reject unknown fields, so dropped).
- OpenHands adapter: `FromEventDecision` emits top-level
  `additionalContext` on SessionStart, UserPromptSubmit.
- OpenCode adapter: `FromEventDecision` + bridge plugin deliver context
  via `tool.execute.after` output and
  `experimental.chat.system.transform` (session start).
- Shell SDK: `nerv_context <text>` queues model-facing context; the next
  `nerv_allow` / `nerv_warn` / `nerv_block` / `nerv_rewrite` carries it as
  `context` (repeated calls join with newlines; empty text ignored).
- **`nervd --adapter <cli>`** (codex, gemini, opencode, openhands): explicit adapter routing in stateless mode. Input decodes and
  the decision encodes through that CLI's translator with the real
  native event name, so `context` reaches the model. Unknown adapter
  exits 2. Without the flag behavior is unchanged (claude dialect for
  `hook_event_name` input), keeping already-installed hooks working.
- `nerv install` writes `nervd --adapter openhands`; legacy bare `nervd` entries are upgraded on reinstall.
- OpenCode bridge: session start routes through the plugin `event`
  hook (`session.created`); tool hooks send typed events; invokes
  `nervd --adapter opencode` (previously `nervd dispatch --format json`,
  which nervd never accepted).
- OpenCode bridge: all bus events (`session.deleted`, `session.idle`,
  `session.error`, `session.compacted`, `file.edited`, `permission.asked`,
  `permission.replied`, `tui.prompt.append`, `todo.updated`) now route
  through the `event` hook and dispatch on `event.type`; the dead
  top-level keys v1 never called are removed. Payload fields follow the
  runtime shapes (`properties.sessionID`/`info.id`, `file`, `reply`,
  `text`, `todos`). `permission.replied` maps only `reject` to
  PermissionDenied; `todo.updated` reports each completed todo once.
  `event` hook errors are logged, not thrown.
- OpenCode supported range pinned to plugin API v1 (`>=1.19.3 <2.0.0`);
  `versions.yaml` opencode entry now `opencode-ai` via npm at `1.19.3`
  (was Go install of 0.3-0.5, which predate the plugin API). v2.x uses a
  different plugin API and is not supported.
- `nerv install` also writes `nervd --adapter gemini` (was
  `nerv-adapter gemini <event>`) and `nervd --adapter codex` (was
  `nerv dispatch`); neither legacy command resolved to a binary. Legacy
  entries are upgraded on reinstall.
- Goose has no model-context channel and drop it; see
  capability-matrix for sources.
- Proto `Decision.context = 5`; Go/TS/Python/PHP stubs regenerated;
  Python and TS handler `Decision` types gain `context`.
- Handler contract: documents `context`; default timeout corrected to
  10s (was documented as 30s).

### Docs

- `docs/parity.md`: one-page harness parity summary (versions,
  entrypoint, block/warn/rewrite/context) with flagged gaps for
  OpenHands, OpenCode, Goose, Codex.

### Project rename

- **`uhp` → `nerv`** — wire-breaking rename across module path, binary
  names, env vars, config dirs, and SDK packages.
  - Module: `hop.top/uhp` → `hop.top/nerv`
  - Binaries: `uhp` → `nerv`, `uhpd` → `nervd`
  - Config dir: `.uhp/` → `.nerv/`
  - Env prefix: `UHP_*` → `NERV_*` (e.g. `UHP_SOCKET` → `NERV_SOCKET`)
  - Wire field: `uhp_version` → `nerv_version` in envelope schema
  - Brand: "Universal Hooks Protocol" replaces "Universal Hook Protocol"
    where applicable; `nerv` is the project name and ships the canonical
    implementation.

### Polyglot SDKs

- **5 SDKs scaffolded under `sdk/`** — first-class client libraries that
  dispatch envelopes through `nervd` over a Unix socket.
  - `sdk/go` — Go client (`hop.top/nerv/sdk/go`)
  - `sdk/ts` — TypeScript / ConnectRPC (`@nerv/sdk`)
  - `sdk/py` — Python / gRPC (`nerv-sdk`)
  - `sdk/rs` — Rust / tonic (`nerv-sdk`)
  - `sdk/php` — PHP / google/protobuf transport stub (`hop-top/nerv`)
- Cross-SDK dispatch parity smoke test landed under `test/parity/`
  exercises Go + TS + Python against a single `nervd` instance.

### Daemon

- **`nervd` refactored to ConnectRPC** — serves the `EventService` over a
  Unix domain socket with `h2c` (HTTP/2 cleartext). `NERV_SOCKET` selects
  the socket path; default lives under `$XDG_RUNTIME_DIR/nerv/`. SDKs in
  every supported language speak the same wire format.
- `nerv daemon start | stop | status | reload` manage the lifecycle;
  `reload` sends `SIGHUP` to hot-swap `hooks.yaml`.

### Release pipeline

- Adopt `release-please` (linked-versions group `nerv` over the 5
  components) gated by a `hop-top/release-bot` App-token mint to bypass
  the CODEOWNERS self-approval block on the manifest.
- Adopt `hop-top/.github` reusable `publish-on-tag@v0` workflow with an
  ecosystems map that fans `*/v*` tags out to Go (goreleaser), npm, PyPI,
  crates.io, and Packagist, with read-only mirror repos under
  `hop-top/<component>`.
- Single-file `release.yaml` removed.
- CI workflow rewired (`ci.yaml` → `ci.yml`) with per-ecosystem jobs
  (`go`, `ts`, `py`, `rs`, `php`) and a `proto` job that runs
  `buf lint` + `buf format -d --exit-code`.
