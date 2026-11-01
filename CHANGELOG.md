# Changelog — nerv

All notable changes to nerv land here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows
[SemVer](https://semver.org/) with `alpha.N` prerelease channel during 0.x.

This file is owned by release-please from `0.1.0-alpha.0` onward — the
`## Unreleased` block below documents the in-flight changes that will land
in the first tagged release.

## Unreleased

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
- Vibe adapter: `FromEventDecision` emits
  `hook_specific_output.additional_context` on post_tool
  (ToolResultEvent).
- OpenCode adapter: `FromEventDecision` + bridge plugin deliver context
  via `tool.execute.after` output and
  `experimental.chat.system.transform` (session start).
- **`nervd --adapter <cli>`** (codex, gemini, opencode, openhands,
  vibe): explicit adapter routing in stateless mode. Input decodes and
  the decision encodes through that CLI's translator with the real
  native event name, so `context` reaches the model. Unknown adapter
  exits 2. Without the flag behavior is unchanged (claude dialect for
  `hook_event_name` input), keeping already-installed hooks working.
- `nerv install` writes `nervd --adapter openhands` / `nervd --adapter
  vibe`; legacy bare `nervd` entries are upgraded on reinstall.
- OpenCode bridge: session start routes through the plugin `event`
  hook (`session.created`); tool hooks send typed events; invokes
  `nervd --adapter opencode` (previously `nervd dispatch --format json`,
  which nervd never accepted).
- Known gap: gemini (`nerv-adapter gemini <event>`) and codex
  (`nerv dispatch`) install commands do not resolve to nervd, so their
  context is not delivered end to end yet.
- Goose and Aider have no model-context channel and drop it; see
  capability-matrix for sources.
- Proto `Decision.context = 5`; Go/TS/Python/PHP stubs regenerated;
  Python and TS handler `Decision` types gain `context`.
- Handler contract: documents `context`; default timeout corrected to
  10s (was documented as 30s).

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
