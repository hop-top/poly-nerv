# Capability matrix — per-adapter event coverage

> Per-harness support and known bugs at a glance: [parity.md](parity.md).

"Universal" with receipts. nerv claims one hook definition runs on
every AI coding CLI; this matrix is the unvarnished count. Each cell
declares whether the named adapter delivers the named event natively,
synthesizes it from another signal, or leaves it uncovered. Sources:
[`spec/events.yaml`](../spec/events.yaml) + each
[`adapters/<name>/capabilities.yaml`](../adapters/).

## Coverage summary

Denominator = 26 native events (the per-adapter scope). The 10
nerv-internal events — 4 extension (Gemini/OpenCode), 2 derived
(MemoryRead/Write), 4 bus (`inbox_message_*`) — are not part of
adapter coverage; see the [Out of scope](#out-of-scope-nerv-internal-events)
appendix.

| Adapter   | Native | Synthesized | Uncovered | Coverage |
|-----------|-------:|------------:|----------:|---------:|
| claude    | 26     | 0           | 0         | 26/26 (100%) |
| gemini    | 8      | 16          | 2         | 24/26 (92%) |
| opencode  | 10     | 13          | 3         | 23/26 (88%) |
| openhands | 5      | 15          | 6         | 20/26 (77%) |
| codex     | 3      | 12          | 11        | 15/26 (58%) |
| goose     | 8      | 0           | 18        | 8/26 (31%) |

Native count = events listed under `native:` in each
`capabilities.yaml`. Synthesized = events listed under `synthesized:`
or `close:` (close-but-not-fully-native semantic mappings are flagged
`~`). Uncovered = events listed under `unsupported:` or absent from
the file.

## Matrix

Rows grouped by category (per
[`.agents/references/event-catalog.md`](../.agents/references/event-catalog.md)).
Columns ordered by coverage, most → least.

| Event                | claude | gemini | opencode | openhands | codex | goose |
|----------------------|:------:|:------:|:--------:|:---------:|:-----:|:-----:|
| **session**          |        |        |          |           |       |       |
| SessionStart         |   ✓    |   ✓    |    ✓     |     ✓     |   ✓   |   ✓   |
| SessionEnd           |   ✓    |   ✓    |    ✓     |     ✓     |   -   |   ✓   |
| Stop                 |   ✓    |   ~    |    ✓     |     ~     |   ✓   |   ✓   |
| StopFailure          |   ✓    |   -    |    ✓     |     ~     |   -   |   -   |
| **prompt**           |        |        |          |           |       |       |
| UserPromptSubmit     |   ✓    |   ~    |    ~     |     ✓     |   ✓   |   ✓   |
| **tool**             |        |        |          |           |       |       |
| PreToolUse           |   ✓    |   ✓    |    ✓     |     ✓     |   ~   |   ✓   |
| PostToolUse          |   ✓    |   ✓    |    ✓     |     ✓     |   ~   |   ✓   |
| PostToolUseFailure   |   ✓    |   ~    |    -     |     ~     |   ~   |   ✓   |
| **permission**       |        |        |          |           |       |       |
| PermissionRequest    |   ✓    |   ~    |    ✓     |     -     |   ~   |   -   |
| PermissionDenied     |   ✓    |   ~    |    ✓     |     ~     |   ~   |   -   |
| **notification**     |        |        |          |           |       |       |
| Notification         |   ✓    |   ✓    |    ~     |     -     |   ~   |   -   |
| **agent**            |        |        |          |           |       |       |
| SubagentStart        |   ✓    |   ✓    |    ~     |     ~     |   -   |   -   |
| SubagentStop         |   ✓    |   ✓    |    ~     |     ~     |   -   |   -   |
| TeammateIdle         |   ✓    |   -    |    -     |     -     |   -   |   -   |
| **task**             |        |        |          |           |       |       |
| TaskCreated          |   ✓    |   ~    |    ~     |     ~     |   -   |   -   |
| TaskCompleted        |   ✓    |   ~    |    ~     |     ~     |   -   |   -   |
| **context**          |        |        |          |           |       |       |
| InstructionsLoaded   |   ✓    |   ~    |    ~     |     ~     |   ~   |   -   |
| ConfigChange         |   ✓    |   ~    |    ~     |     -     |   ~   |   -   |
| CwdChanged           |   ✓    |   ~    |    ~     |     ~     |   ~   |   -   |
| PreCompact           |   ✓    |   ✓    |    -     |     ~     |   -   |   -   |
| PostCompact          |   ✓    |   ~    |    ✓     |     ~     |   -   |   -   |
| **file**             |        |        |          |           |       |       |
| FileChanged          |   ✓    |   ~    |    ✓     |     ~     |   ~   |   ✓   |
| **worktree**         |        |        |          |           |       |       |
| WorktreeCreate       |   ✓    |   ~    |    ~     |     -     |   ~   |   -   |
| WorktreeRemove       |   ✓    |   ~    |    ~     |     -     |   ~   |   -   |
| **mcp**              |        |        |          |           |       |       |
| Elicitation          |   ✓    |   ~    |    ~     |     ~     |   -   |   -   |
| ElicitationResult    |   ✓    |   ~    |    ~     |     ~     |   -   |   -   |

Legend: `✓` native | `~` synthesized (derived from another signal,
including `close:` semantic-mismatch mappings) | `-` not covered.

## Context injection and delivery

Per-harness delivery (block, warn, rewrite, context channel, entrypoint,
known bugs) lives in [parity.md](parity.md); the context contract is
[handler contract § Context](../spec/handler-contract.md#context). Not
repeated here to avoid drift.

## How to read this matrix

Each cell is grounded in the adapter's `capabilities.yaml`. A `✓`
means the upstream CLI emits a lifecycle hook that the adapter can
translate one-to-one. A `~` means the adapter recovers the event by
correlating other signals — shell-command regex (`CwdChanged`,
`WorktreeCreate`/`Remove`), MCP tool-name match (`Elicitation*`),
file-watch daemons (`FileChanged`, `ConfigChange` on Codex). A `-` means the
upstream CLI exposes no usable signal and the adapter does not
attempt synthesis.

Recurring blockers (lifted from each `capabilities.yaml`'s
`unsupported:` list and `notes:`):

- **`TeammateIdle`** — Claude-only multi-agent primitive. Every
  other CLI has no equivalent of an agent-team peer going idle.
- **Goose's gap (18 uncovered).** Goose delivers 8 plugin hooks
  (`SessionStart`, `SessionEnd`, `Stop`, `UserPromptSubmit`, `PreToolUse`,
  `PostToolUse`, `PostToolUseFailure`, `AfterFileEdit` as `FileChanged`).
  Its hook list has no permission, notification, subagent, task,
  compaction, cwd, worktree, instruction or MCP events, and nerv installs
  no other route. See [parity](parity.md#flag-goose).
- **Codex CLI's gap (11 uncovered).** Codex exposes 3 native hooks
  (`SessionStart`, `UserPromptSubmit`, `Stop`). Anything subagent-,
  task-, compaction-, or MCP-shaped requires features Codex doesn't
  emit. `Notification` is reachable only by wrapping the Codex
  binary. See [`adapters/codex/capabilities.yaml`](../adapters/codex/capabilities.yaml)
  for technique + cost annotations (`shell`, `daemon`, `binary_wrap`).
- **OpenCode bus events** — `session.*`, `file.edited`, `permission.*`,
  `todo.updated`, `tui.prompt.append` reach the bridge only through the
  plugin `event` hook (`{event: {type, properties}}`); v1 has no
  per-event top-level plugin keys. `permission.replied` carries
  `reply: once|always|reject`; only `reject` maps to `PermissionDenied`.
  The `permission.ask` hook is declared in `packages/plugin/src/index.ts`
  but never triggered by the runtime (v1.19.3), so permission events are
  observation-only. `todo.updated` fires `TaskCompleted` once per newly
  completed todo id. Blocking is `throw` from `tool.execute.before`.
  On plugin API v2 (OpenCode 2.x) the bridge subscribes with
  `ctx.event.subscribe()` (`{type, data}`), maps `session.execution.*`
  to `Stop`/`StopFailure`, `session.compaction.ended` to `PostCompact`,
  and derives `FileChanged`/`TaskCompleted` from edit/write/patch and
  todowrite completions; see [parity](parity.md#flag-opencode).
- **OpenCode `PreCompact`** — OpenCode emits `session.compacted`
  (post-) but no pre-compaction signal.
- **OpenHands worktree + notifications + config** — the SDK PR
  ([software-agent-sdk #1467](https://github.com/OpenHands/software-agent-sdk/pull/1467))
  doesn't yet plumb these; flagged for future work.

For per-event-per-adapter detail (technique, regex, cost), read the
adapter's `capabilities.yaml` directly — each entry under
`synthesized:` carries `source:`/`via:`/`pattern:`/`notes:` fields.

## Out of scope (nerv-internal events)

The 10 events below are part of the nerv superset (36 total in
[`spec/events.yaml`](../spec/events.yaml)) but are not per-adapter
obligations:

| Event                  | Origin    | Source / synthesis                                          |
|------------------------|-----------|-------------------------------------------------------------|
| `BeforeToolSelection`  | extension | gemini-cli only; other adapters ignore                      |
| `LspDiagnostics`       | extension | opencode-only                                               |
| `SessionDiff`          | extension | opencode-only                                               |
| `SessionStatus`        | extension | opencode-only                                               |
| `MemoryRead`           | derived   | dispatcher-synthesized from `PreToolUse` + memory-path regex |
| `MemoryWrite`          | derived   | dispatcher-synthesized from `PreToolUse` + memory-path regex |
| `inbox_message_added`     | bus | kit-bus item lifecycle; not adapter-sourced |
| `inbox_message_updated`   | bus | kit-bus item lifecycle; not adapter-sourced |
| `inbox_message_promoted`  | bus | kit-bus item lifecycle; not adapter-sourced |
| `inbox_message_dropped`   | bus | kit-bus item lifecycle; not adapter-sourced |

Extension events appear natively in their source adapter and are
omitted (or synthesized only if it makes sense) elsewhere. Derived +
bus events fire from the dispatcher / kit-bus, not from any per-CLI
lifecycle.

## Caveats

- **`gemini` and `opencode` count drift vs. `docs/research/mappings.md`.**
  `mappings.md` records `gemini StopFailure` and
  `opencode PostToolUseFailure` as workarounds, but the corresponding
  `capabilities.yaml` files don't declare them. This matrix follows
  the YAML (the canonical adapter contract) and renders both `-`.
  `mappings.md` is a higher-level research note and may pre-date the
  current `capabilities.yaml` shape.
- **`close:` collapses into `~`.** The matrix uses three symbols
  (`✓ / ~ / -`). The `close:` block in capabilities.yaml (semantic
  mismatch, usable) renders as `~`. Read the underlying file for the
  semantic-mismatch note (e.g. Gemini's `BeforeModel → UserPromptSubmit`
  fires per model call, not per user prompt).

## Regeneration

Last regenerated: 2026-05-27.

Source of truth: [`spec/events.yaml`](../spec/events.yaml) +
[`adapters/*/capabilities.yaml`](../adapters/). No generator is
shipped yet; this matrix is hand-maintained against those files.
Regenerate by re-reading each `capabilities.yaml` and re-tallying.
