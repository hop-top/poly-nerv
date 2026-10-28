# Capability matrix — per-adapter event coverage

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
| goose     | 11     | 11          | 4         | 22/26 (85%) |
| openhands | 5      | 15          | 6         | 20/26 (77%) |
| codex     | 3      | 12          | 11        | 15/26 (58%) |
| vibe      | 6      | 8           | 12        | 14/26 (54%) |
| aider     | 0      | 5           | 20 (+1 undeclared) | 5/26 (19%) |

Native count = events listed under `native:` in each
`capabilities.yaml`. Synthesized = events listed under `synthesized:`
or `close:` (close-but-not-fully-native semantic mappings are flagged
`~`). Uncovered = events listed under `unsupported:` or absent from
the file.

## Matrix

Rows grouped by category (per
[`.agents/references/event-catalog.md`](../.agents/references/event-catalog.md)).
Columns ordered by coverage, most → least.

| Event                | claude | gemini | opencode | goose | openhands | codex | vibe | aider |
|----------------------|:------:|:------:|:--------:|:-----:|:---------:|:-----:|:----:|:-----:|
| **session**          |        |        |          |       |           |       |      |       |
| SessionStart         |   ✓    |   ✓    |    ✓     |   ✓   |     ✓     |   ✓   |  ~   |   ~   |
| SessionEnd           |   ✓    |   ✓    |    ✓     |   ✓   |     ✓     |   -   |  ~   |   ~   |
| Stop                 |   ✓    |   ~    |    ✓     |   ✓   |     ~     |   ✓   |  ~   |   -   |
| StopFailure          |   ✓    |   -    |    ✓     |   ✓   |     ~     |   -   |  -   |   -   |
| **prompt**           |        |        |          |       |           |       |      |       |
| UserPromptSubmit     |   ✓    |   ~    |    ~     |   ~   |     ✓     |   ✓   |  ✓   |   -   |
| **tool**             |        |        |          |       |           |       |      |       |
| PreToolUse           |   ✓    |   ✓    |    ✓     |   ~   |     ✓     |   ~   |  ✓   |   ~   |
| PostToolUse          |   ✓    |   ✓    |    ✓     |   ~   |     ✓     |   ~   |  ✓   |   ~   |
| PostToolUseFailure   |   ✓    |   ~    |    -     |   ~   |     ~     |   ~   |  -   |   ?   |
| **permission**       |        |        |          |       |           |       |      |       |
| PermissionRequest    |   ✓    |   ~    |    ✓     |   ✓   |     -     |   ~   |  ~   |   -   |
| PermissionDenied     |   ✓    |   ~    |    ✓     |   ✓   |     ~     |   ~   |  ~   |   -   |
| **notification**     |        |        |          |       |           |       |      |       |
| Notification         |   ✓    |   ✓    |    ~     |   ✓   |     -     |   ~   |  -   |   -   |
| **agent**            |        |        |          |       |           |       |      |       |
| SubagentStart        |   ✓    |   ✓    |    ~     |   ✓   |     ~     |   -   |  ~   |   -   |
| SubagentStop         |   ✓    |   ✓    |    ~     |   ✓   |     ~     |   -   |  -   |   -   |
| TeammateIdle         |   ✓    |   -    |    -     |   -   |     -     |   -   |  -   |   -   |
| **task**             |        |        |          |       |           |       |      |       |
| TaskCreated          |   ✓    |   ~    |    ~     |   ~   |     ~     |   -   |  -   |   -   |
| TaskCompleted        |   ✓    |   ~    |    ~     |   ✓   |     ~     |   -   |  -   |   -   |
| **context**          |        |        |          |       |           |       |      |       |
| InstructionsLoaded   |   ✓    |   ~    |    ~     |   ~   |     ~     |   ~   |  -   |   -   |
| ConfigChange         |   ✓    |   ~    |    ~     |   -   |     -     |   ~   |  ✓   |   -   |
| CwdChanged           |   ✓    |   ~    |    ~     |   ~   |     ~     |   ~   |  -   |   -   |
| PreCompact           |   ✓    |   ✓    |    -     |   ~   |     ~     |   -   |  ✓   |   -   |
| PostCompact          |   ✓    |   ~    |    ✓     |   ✓   |     ~     |   -   |  ✓   |   -   |
| **file**             |        |        |          |       |           |       |      |       |
| FileChanged          |   ✓    |   ~    |    ✓     |   ~   |     ~     |   ~   |  -   |   ~   |
| **worktree**         |        |        |          |       |           |       |      |       |
| WorktreeCreate       |   ✓    |   ~    |    ~     |   ~   |     -     |   ~   |  -   |   -   |
| WorktreeRemove       |   ✓    |   ~    |    ~     |   ~   |     -     |   ~   |  -   |   -   |
| **mcp**              |        |        |          |       |           |       |      |       |
| Elicitation          |   ✓    |   ~    |    ~     |   -   |     ~     |   -   |  ~   |   -   |
| ElicitationResult    |   ✓    |   ~    |    ~     |   -   |     ~     |   -   |  ~   |   -   |

Legend: `✓` native | `~` synthesized (derived from another signal,
including `close:` semantic-mismatch mappings) | `-` not covered |
`?` not declared in `capabilities.yaml` (treat as uncovered until the
file is amended).

## How to read this matrix

Each cell is grounded in the adapter's `capabilities.yaml`. A `✓`
means the upstream CLI emits a lifecycle hook that the adapter can
translate one-to-one. A `~` means the adapter recovers the event by
correlating other signals — shell-command regex (`CwdChanged`,
`WorktreeCreate`/`Remove`), MCP tool-name match (`Elicitation*`),
file-watch daemons (`FileChanged`, `ConfigChange` on Codex), or
middleware/callbacks (`SessionStart` on Vibe). A `-` means the
upstream CLI exposes no usable signal and the adapter does not
attempt synthesis.

Recurring blockers (lifted from each `capabilities.yaml`'s
`unsupported:` list and `notes:`):

- **`TeammateIdle`** — Claude-only multi-agent primitive. Every
  other CLI has no equivalent of an agent-team peer going idle.
- **Codex CLI's gap (11 uncovered).** Codex exposes 3 native hooks
  (`SessionStart`, `UserPromptSubmit`, `Stop`). Anything subagent-,
  task-, compaction-, or MCP-shaped requires features Codex doesn't
  emit. `Notification` is reachable only by wrapping the Codex
  binary. See [`adapters/codex/capabilities.yaml`](../adapters/codex/capabilities.yaml)
  for technique + cost annotations (`shell`, `daemon`, `binary_wrap`).
- **OpenCode `PreCompact`** — OpenCode emits `session.compacted`
  (post-) but no pre-compaction signal.
- **OpenHands worktree + notifications + config** — the SDK PR
  ([software-agent-sdk #1467](https://github.com/OpenHands/software-agent-sdk/pull/1467))
  doesn't yet plumb these; flagged for future work.
- **Vibe (Mistral)** — no file/shell/worktree lifecycle; Vibe is a
  multi-turn chat surface, not a file-editing CLI, so 12 of the 26
  events have no natural source.
- **Aider** — Aider has no hook protocol. All 5 covered events are
  synthesized by an out-of-band `nerv` bridge script attached to
  `lint-cmd` / `test-cmd`. Everything else is uncovered; one event
  (`PostToolUseFailure`) is undeclared in
  [`adapters/aider/capabilities.yaml`](../adapters/aider/capabilities.yaml)
  and rendered `?` pending a file amendment.

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

- **`aider/capabilities.yaml` omits `PostToolUseFailure`.** It is
  not in `synthesized:` and not in `unsupported:`. Rendered `?` in
  the matrix. Aider has no native failure event and the bridge
  script does not currently synthesize one; the file should declare
  it explicitly.
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
