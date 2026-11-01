# Harness parity — what nerv supports per CLI, and what is broken

Single place to scan cross-harness support. Event-by-event coverage lives
in the [capability matrix](capability-matrix.md); the normative decision
and context contract lives in the
[handler contract](../spec/handler-contract.md). This page owns the
per-harness delivery summary and the known-gap detail. Other docs link
here instead of restating it.

Every cell is grounded in `adapters/<x>/capabilities.yaml` or adapter
code; upstream behavior is cited where a cell depends on it.

## Summary

| Harness | Versions (range / CI pin) | Entrypoint | Block | Warn | Rewrite | Context (events) | Session events | Known gaps |
|---|---|---|:-:|:-:|:-:|---|---|---|
| claude | `>=1.0.0` / 1.0.18, 1.1.4, 1.2.1 | `nervd` (or `--adapter claude`); stdin JSON | ✓ | ◐ stderr | ✓ PreToolUse, PermissionRequest | ✓ SessionStart, SubagentStart, UserPromptSubmit, PreToolUse, PostToolUse, PostToolUseFailure, Stop, SubagentStop | ✓ Start, End, Stop | none; reference CLI |
| gemini | no range / 0.3.2, 0.4.1, 0.5.0 | `nervd --adapter gemini`; stdin JSON | ✓ | ✓ `systemMessage` | ? emits `tool_input` | ✓ SessionStart, BeforeAgent, AfterTool | ◐ Start, End native; Stop = SessionEnd | [gemini](#flag-gemini) |
| codex | `>=0.1.0` / 0.40.0, 0.41.0, 0.42.0 | `nervd --adapter codex`; stdin JSON | ◐ | ◐ stderr | ? claude encoding | ◐ SessionStart, UserPromptSubmit, PreToolUse, PostToolUse | ◐ Start, Stop; no End | [codex](#flag-codex) |
| opencode | `>=1.19.3 <2.0.0 \|\| >=2.2.0 <3.0.0` / 1.19.3, 2.2.5 | `nervd --adapter opencode`; bridge plugin | ✓ PreToolUse only | ◐ bridge log | ✗ ignored | ✓ PostToolUse, SessionStart | ✓ Start, End, Stop | ⚠ [opencode](#flag-opencode) |
| goose | `>=1.55.0` / 1.55.0 | `nervd --adapter goose`; plugin `hooks.json` (`~/.agents/plugins/nerv/hooks/hooks.json`), stdin JSON | ◐ PreToolUse, Stop | ✗ stderr only | ✗ | ✗ no channel | ✓ Start, End, Stop | ⚠ [goose](#flag-goose) |
| openhands | no range, no pin | `nervd --adapter openhands`; stdin JSON | ⚠ via exit 2 | ◐ `reason` + stderr | ✗ | ◐ SessionStart, UserPromptSubmit | ◐ Start, End; Stop needs core change | ⚠ [openhands](#flag-openhands) |

Vibe (Mistral) and Aider are not supported: no hook protocol, or the
adapter did not reach parity.

Legend: `✓` supported | `◐` partial (see gap) | `✗` none or no host
channel | `⚠` known bug, see detail | `?` adapter emits it, host
honoring not verified.

Column notes:

- **Block** — host actually refuses the action when nerv blocks.
- **Warn** — message reaches the user. Claude and Codex have no JSON warn
  channel; `nervd` prints `nervd: warn: <msg>` to stderr.
- **Context** — model-facing `context` delivery; channel detail in the
  [handler contract](../spec/handler-contract.md#context). Dropped
  silently where there is no channel.
- **Entrypoint** — `nerv install` writes the flag into the host's hook
  config; without `--adapter`, input with `hook_event_name` is answered
  as Claude Code. Unknown adapter exits 2.
- Event counts per harness: [capability matrix](capability-matrix.md#coverage-summary).

## Known gaps

### <a id="flag-openhands"></a>⚠ OpenHands — wrong decision key, block still honored via exit code

Adapter: `adapters/openhands/translate.go` `FromEventDecision`.

- It emits `{"action":"allow|block","reason":...,"additionalContext":...}`.
  OpenHands reads `decision`, not `action`, and accepts only
  `allow` | `deny`
  ([`executor.py`](https://github.com/OpenHands/software-agent-sdk/blob/main/openhands-sdk/openhands/sdk/hooks/executor.py),
  `decision_value = output_data.get("decision")`; `HookDecision` is
  `ALLOW`/`DENY`). The `action` key is ignored.
- Block: nerv exits 2, and OpenHands sets `blocked = (returncode == 2)`.
  The tool call or stop is therefore blocked correctly. `reason` is
  parsed from stdout regardless of exit code, so the message survives.
- Lost: the JSON verdict never fires (`decision` stays unset), so the
  only block path is the exit code; any host path that consumes
  `decision` without checking `blocked` sees nothing. Warn and rewrite
  both map to `allow` (no `decision` emitted), so rewrite is dropped
  and warn text is only `reason` plus the stderr line.
- Fix direction (not done here): emit `decision:"deny"` on block.
- Context: top-level `additionalContext` on SessionStart and
  UserPromptSubmit works as documented; PreToolUse/PostToolUse results
  are not consumed.
- Stop is `close:` in `capabilities.yaml` ("API-ready, needs core loop
  change").

### <a id="flag-opencode"></a>⚠ OpenCode — two plugin APIs (v1, v2); permissions observation-only; rewrite ignored

- Supported: plugin API v1, `>=1.19.3 <2.0.0` (verified against
  sst/opencode v1.19.3), and plugin API v2, `>=2.2.0 <3.0.0` (verified
  against anomalyco/opencode v2.2.5; also run against 2.2.1). v2 does not
  load v1 plugins (`Plugin.define`/`setup`), so the installer writes the
  bridge matching `opencode --version` and removes the other one.
  Unparseable versions and 3.x fail the install. CI pins:
  `opencode-ai@1.19.3` (npm) and 2.2.5 (GitHub release; v2 is not
  published to npm under `opencode-ai`).
- v2 bridge (`adapters/opencode/shim_v2.go`), installed as
  `.opencode/nerv-bridge/index.ts` plus a `plugins` entry
  (`./.opencode/nerv-bridge`) in `opencode.json`. Not under
  `.opencode/plugins/`: that directory is auto-discovered, so a config
  entry pointing into it makes OpenCode 2.2.5 load the same plugin twice
  (two "loading plugin" log lines; the loader rejects duplicate ids). Same wire shape and decisions as v1; v2 differences absorbed in
  the bridge: tool ids `shell`/`subagent` are sent as `bash`/`task`,
  `path` args also as `filePath`, `todo.updated`/`file.edited`/
  `session.idle` have no v2 bus event and are derived (todowrite, edit/
  write/patch completions, `session.execution.*`), and session start is
  announced from `session.created` or the first model request (the
  `session.created` event is not delivered to a plugin loaded after the
  session exists). A failed tool also reports `error` (→
  `PostToolUseFailure`), which v1 never does.
- `permission.ask` is declared in `packages/plugin/src/index.ts` but
  never triggered by the runtime (v1.19.3). `PermissionRequest` and
  `PermissionDenied` come from bus events (`permission.asked`,
  `permission.replied` with `reject`) through the plugin `event` hook;
  they observe and cannot allow or deny.
- Block works only by throwing from `tool.execute.before`
  (`adapters/opencode/shim.go`; v2: `ctx.tool.hook("execute.before")`,
  `shim_v2.go`, the message reaches the model).
- Rewrite is ignored: the shim logs "rewrite decision ignored".
- Context: `tool.execute.after` (appended to `output.output`; v2:
  appended to `result.content`) and `session.created` (via
  `experimental.chat.system.transform`; v2: `ctx.session.hook("context")`
  pushes a text part onto `event.system`). No channel on
  `tool.execute.before` (args only).
- No `PreCompact` (only post-compaction `session.compacted`).
  `PostToolUseFailure` is undeclared and renders `-`.

### <a id="flag-goose"></a>⚠ Goose — observation-only except PreToolUse/Stop block; no context channel

- Fixed: the installer wrote an MCP extension `nerv-bridge` running
  `nerv dispatch goose`, which does not exist. It now writes a Goose
  plugin hooks file, `~/.agents/plugins/nerv/hooks/hooks.json`, whose
  commands are `nervd --adapter goose`, for SessionStart, SessionEnd,
  Stop, UserPromptSubmit, PreToolUse, PostToolUse, PostToolUseFailure,
  AfterFileEdit. Legacy `nerv dispatch goose` and bare `nervd` commands in
  that file are upgraded in place. Verified against goose 1.55.0 with a
  real run (`TestRealGoose_*`, set `NERV_GOOSE_BIN`): a nerv block on
  PreToolUse denies the tool call; a block on Stop keeps the turn going.
- Block: only PreToolUse and Stop (`hooks.md` "Blocking a Tool Call").
  nervd answers `{"decision":"block","reason":...}` on stdout, exit 0;
  the reason reaches the model. A block on any other event is ignored by
  Goose, so nervd prints nothing.
- Stop block is capped by Goose at 8 consecutive blocks
  (`GOOSE_STOP_HOOK_BLOCK_CAP`), after which it ends the turn.
- No context channel: Goose reads stdout only for the decision and, on
  SessionStart, a user-visible `banner` that is stored with
  `agent_visible=false` (`crates/goose/src/hooks/mod.rs`
  `emit_collecting_banners`, `agents/state_machine/session.rs`
  `session_start_message`). `context` is dropped. Warn text goes to
  stderr only; rewrite is dropped.
- Fail-open by default: a hook that crashes, times out, or prints
  anything but a decision on stdout lets the tool call proceed
  (`on_failure` defaults to `allow`); the installer does not set
  `on_failure: block`.
- Coverage is 8/26, all native plugin hooks: SessionStart, SessionEnd,
  Stop, UserPromptSubmit, PreToolUse, PostToolUse, PostToolUseFailure,
  AfterFileEdit (as FileChanged). Not delivered, and listed `unsupported`
  in `capabilities.yaml`: StopFailure, PermissionRequest, PermissionDenied,
  Notification, SubagentStart, SubagentStop (Goose never emits subagent
  hooks), TeammateIdle, TaskCreated, TaskCompleted, InstructionsLoaded,
  ConfigChange, CwdChanged, PreCompact, PostCompact, WorktreeCreate,
  WorktreeRemove, Elicitation, ElicitationResult. No synthesized events:
  nothing turns a delivered hook into one.
- Goose hook events deliberately unmapped (`hooks.md` "Supported Events",
  `crates/goose/src/hooks/mod.rs` `HookEvent`): PreToolUseResult
  (observation of the PreToolUse outcome), BeforeReadFile (no nerv event),
  BeforeShellExecution and AfterShellExecution (duplicate the tool events;
  mapping them to CwdChanged/Worktree* needs shell-command parsing).
- Tool names are Goose's (`shell`, `write`, `edit`), not Claude's;
  handler `match` patterns must use them.
- An earlier install left a `nerv-bridge` entry in
  `~/.config/goose/config.yaml`; the installer only manages the hooks
  file, so remove that entry by hand.
- Pinned to 1.55.0 (`versions.yaml`, `cli_version_range`); earlier
  releases are unverified.

### <a id="flag-codex"></a>◐ Codex — context events are a subset; strict output schemas

- Native hooks: SessionStart, UserPromptSubmit, Stop. PreToolUse and
  PostToolUse are synthesized via shell matcher (Bash-only,
  `apply_patch` incomplete); 11 events are unsupported.
- Context: SessionStart, UserPromptSubmit, PreToolUse, PostToolUse. Stop,
  SubagentStop and compaction outputs reject unknown fields, so
  `context` is dropped there (Claude accepts it on Stop).
- Block, warn and rewrite reuse the Claude encoding. Rewrite honoring
  by Codex is not verified.

### <a id="flag-gemini"></a>◐ Gemini — approximations

- UserPromptSubmit is `BeforeModel` (fires per model call, not per user
  prompt); Stop is `SessionEnd` (graceful exit only); PostCompact is
  detected from a marker in `AfterModel` (`close:` entries).
- Rewrite is emitted as `hookSpecificOutput.tool_input`; host
  honoring not verified.
- `StopFailure` is undeclared in `capabilities.yaml` (research notes
  list a workaround) and renders `-`.

## Sources

Checked against upstream HEAD, Nov 2026: openai/codex
`codex-rs/hooks/schema/generated/*.command.output.schema.json` and
`codex-rs/hooks/src/events/*.rs`; sst/opencode v1.19.3
`packages/plugin/src/index.ts` (V1 `Hooks`),
`packages/opencode/src/session/tools.ts` and `session/llm/request.ts`
(V2 equivalents: `ctx.tool.hook("execute.after")`,
`ctx.session.hook("context")`, per
`services/www/src/docs/content/build/plugins/migrate-v1.mdx`);
OpenHands/software-agent-sdk `openhands-sdk/openhands/sdk/hooks/`
(`executor.py`, `conversation_hooks.py`); aaif-goose/goose `documentation/docs/guides/context-engineering/hooks.md`.
