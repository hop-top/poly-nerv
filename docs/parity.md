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
| opencode | `>=1.19.3 <2.0.0` / 1.19.3 | `nervd --adapter opencode`; bridge plugin | ✓ PreToolUse only | ◐ bridge log | ✗ ignored | ✓ PostToolUse, SessionStart | ✓ Start, End, Stop | ⚠ [opencode](#flag-opencode) |
| goose | no range, no pin | none (not an `--adapter` route); MCP extension `nerv-bridge` | ◐ PreToolUse, Stop | ◐ decision `message` | ✗ | ✗ no channel | ✓ Start, End, Stop | ⚠ [goose](#flag-goose) |
| openhands | no range, no pin | `nervd --adapter openhands`; stdin JSON | ⚠ via exit 2 | ◐ `reason` + stderr | ✗ | ◐ SessionStart, UserPromptSubmit | ◐ Start, End; Stop needs core change | ⚠ [openhands](#flag-openhands) |
| vibe | no range, no pin | `nervd --adapter vibe`; stdin JSON | ⚠ not honored | ✗ | ✗ | ◐ post_tool only | ◐ synthesized | ⚠ [vibe](#flag-vibe) |
| aider | no range, no `versions.yaml` entry | `nervd --adapter aider lint <file>...` / `test`; argv, never stdin | ◐ post-edit only | ◐ stderr, user only | ✗ | ✗ no channel | ✗ none | ⚠ [aider](#flag-aider) |

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

### <a id="flag-vibe"></a>⚠ Vibe — block not honored; hooks likely inert on current Vibe

Adapter: `adapters/vibe/translate.go`, `adapters/vibe/adapter.go`,
`cmd/nervd/adapter.go` `decodeVibe`. Upstream:
[`SKILL.md`](https://github.com/mistralai/mistral-vibe/blob/main/vibe/plugins/builtins/vibe/skills/vibe/SKILL.md)
(Hooks, "Structured-response schema").

1. **Block shape.** Adapter emits `{"decision":"block",...}` and exits
   2. Vibe's schema is `decision: "allow" | "deny"`; and per its
   exit-code table only exit 0 with a valid JSON object is acted on.
   Any non-zero exit is the "failure path": a UI warning, then the
   action proceeds (fail open). Vibe has no special meaning for exit 2.
   Impact: a nerv `block` is **not** honored. With `strict = true` on a
   `pre_tool` hook the failure escalates to a deny, with the raw
   stdout/stderr text as reason; `post_agent` cannot be strict. nerv's
   installer does not write `strict`.
2. **Inbound decode.** Vibe sends `hook_event_name` (`pre_tool`,
   `post_tool`, `post_agent`). `decodeVibe` reads `event` and expects
   class names (`ToolCallEvent`, ...). Real Vibe input fails with
   `vibe: missing event field` and exit 1, which is also the failure
   path, so nothing is evaluated. Probed:
   `echo '{"hook_event_name":"pre_tool",...}' | nervd --adapter vibe`
   exits 1.
3. **Install format.** The installer appends `[hooks]` and
   `[[hooks.handlers]]` (`event = "ToolCallEvent"`, `timeout = 30000`)
   to `config.toml`. Vibe reads `hooks.toml` with `[[hooks]]` entries
   (`name`, `type = pre_tool|post_tool|post_agent`, `command`,
   `timeout` in seconds as a float). Whether current Vibe reads the
   installed block at all is unverified; by the documented format it
   does not.
4. **Rewrite.** Vibe supports `hook_specific_output.tool_input` on
   `pre_tool`; the adapter maps rewrite to `allow` and drops it.
5. **Warn.** Emitted as `reason`, which Vibe uses only for deny. The
   UI-visible field is `system_message` (not emitted).
6. Context: `hook_specific_output.additional_context` on `post_tool`
   matches upstream; `pre_tool` and `post_agent` have none.

Net: treat Vibe as observation-unverified until items 1-3 are fixed.

### <a id="flag-opencode"></a>⚠ OpenCode — plugin API v1 only; permissions observation-only; rewrite ignored

- Supported: plugin API v1, `>=1.19.3 <2.0.0` (verified against
  sst/opencode v1.19.3). v2.x uses a different plugin API
  (`Plugin.define`/`setup`) and does not load v1 plugins: **unsupported**.
  CI pin is `opencode-ai@1.19.3` only.
- `permission.ask` is declared in `packages/plugin/src/index.ts` but
  never triggered by the runtime (v1.19.3). `PermissionRequest` and
  `PermissionDenied` come from bus events (`permission.asked`,
  `permission.replied` with `reject`) through the plugin `event` hook;
  they observe and cannot allow or deny.
- Block works only by throwing from `tool.execute.before`
  (`adapters/opencode/shim.go`).
- Rewrite is ignored: the shim logs "rewrite decision ignored".
- Context: `tool.execute.after` (appended to `output.output`) and
  `session.created` (via `experimental.chat.system.transform`). No
  channel on `tool.execute.before` (args only).
- No `PreCompact` (only post-compaction `session.compacted`).
  `PostToolUseFailure` is undeclared and renders `-`.

### <a id="flag-aider"></a>⚠ Aider — no hook protocol; post-edit lint/test commands only

- Extension points are `lint-cmd` (FileChanged, edited file as trailing
  argv) and `test-cmd` (PostToolUse, no args), installed as
  `nervd --adapter aider lint|test`. Both run after Aider applied the
  edit (`aider/coders/base_coder.py` `lint_edited`, `cmd_test`), so
  there is no pre-tool blocking: PreToolUse and session events are
  uncovered.
- Block = exit 1 with the message on stdout, which Aider feeds back to
  the model as an error-repair prompt. Allow and warn exit 0; warn text
  goes to stderr, user only. Rewrite is dropped.
- Context: no channel. Output reaches the model only on non-zero exit,
  so context would read as a failure (`aider/commands.py`
  `cmd_test`/`cmd_run`, `aider/linter.py` `run_cmd`); dropped.
- A fresh global `lint-cmd` **replaces Aider's built-in linters for all
  languages**. An existing command is chained as
  `f(){ nervd ... "$@" && <orig> "$@"; }; f`; per-language
  `python: cmd` entries are wrapped per language; YAML list forms are
  rejected.
- No `versions.yaml` entry and no `cli_version_range`.
- `PostToolUseFailure` is absent from `adapters/aider/capabilities.yaml`
  (neither synthesized nor unsupported); the matrix renders it `?`.

### <a id="flag-goose"></a>⚠ Goose — observation-only except PreToolUse/Stop block; bridge command does not resolve

- Goose hooks are observation-only except blocking on PreToolUse and
  Stop. Hook stdout is read only for the decision and, on SessionStart,
  a user-visible `banner` stored with `agent_visible=false`
  (`crates/goose/src/hooks/mod.rs` `emit_collecting_banners`,
  `agents/state_machine/session.rs` `session_start_message`). No context
  channel; `context` is dropped.
- Rewrite maps to `continue` and is dropped.
- Goose is not an `nervd --adapter` route. The installer writes an MCP
  extension `nerv-bridge` with `command: nerv`, `args: [dispatch, goose]`
  (`adapters/goose/adapter.go`), and `nerv dispatch` is not a `nerv`
  subcommand (`nerv dispatch goose` exits 2 with "unknown command").
  The same defect was fixed for Aider, Gemini and Codex; Goose still
  has it, so installed Goose hooks do not reach nervd.
- No `versions.yaml` entry and no `cli_version_range`.

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
(`executor.py`, `conversation_hooks.py`); mistralai/mistral-vibe
`vibe/plugins/builtins/vibe/skills/vibe/SKILL.md` (Hooks);
aaif-goose/goose `documentation/docs/guides/context-engineering/hooks.md`;
Aider-AI/aider `aider/commands.py`.
