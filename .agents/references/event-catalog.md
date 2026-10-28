# Event catalog

Canonical taxonomy. Ground truth:
[`spec/events.yaml`](../../spec/events.yaml).

36 entries grouped by category. Every event declares
`origin: native | derived | extension` (default native).

The README cites "26 canonical events" — that's Claude Code's
PreToolUse-class native count. The total **36** here is the
post-derivation, cross-CLI superset: 26 native + 4 extension
(Gemini/OpenCode) + 2 derived (MemoryRead/Write) + 4 bus
(inbox_message_*).

## Origin legend

- **native** — emitted directly by the CLI's lifecycle.
- **extension** — native to one CLI, synthesized or ignored by others.
  `extension_source:` names the origin CLI.
- **derived** — synthesized by the dispatcher from a native event +
  filter; emitted **in addition** to the source event.

## session

| Event | Blocking | Origin | When |
|---|---|---|---|
| `SessionStart` | no | native | Session begins or resumes. |
| `SessionEnd` | no | native | Session terminates (graceful or forced). |
| `Stop` | yes | native | Model finishes responding for this turn. |
| `StopFailure` | no | native | Turn ends due to API/internal error. |
| `SessionDiff` | no | extension (opencode) | Session code diff computed. |
| `SessionStatus` | no | extension (opencode) | Session status transition. |

## prompt

| Event | Blocking | Origin | When |
|---|---|---|---|
| `UserPromptSubmit` | yes | native | User submits a prompt before model processing. |

## tool

| Event | Blocking | Origin | When |
|---|---|---|---|
| `PreToolUse` | yes | native | Before a tool call executes; handler can block. |
| `PostToolUse` | no | native | After a tool call succeeds. |
| `PostToolUseFailure` | no | native | After a tool call fails. |
| `BeforeToolSelection` | yes | extension (gemini-cli) | Before model picks which tool to call. |

## permission

| Event | Blocking | Origin | When |
|---|---|---|---|
| `PermissionRequest` | yes | native | Permission dialog appears for a tool call. |
| `PermissionDenied` | no | native | Tool call denied by auto-mode classifier or user. |

## notification

| Event | Blocking | Origin | When |
|---|---|---|---|
| `Notification` | no | native | CLI sends a user-facing notification. |

## agent

| Event | Blocking | Origin | When |
|---|---|---|---|
| `SubagentStart` | no | native | Subagent spawned by the main agent. |
| `SubagentStop` | no | native | Subagent finishes execution. |
| `TeammateIdle` | no | native | Agent-team teammate going idle (multi-agent only). |

## task

| Event | Blocking | Origin | When |
|---|---|---|---|
| `TaskCreated` | no | native | Task created via TaskCreate tool or equivalent. |
| `TaskCompleted` | no | native | Task marked completed. |

## context

| Event | Blocking | Origin | When |
|---|---|---|---|
| `InstructionsLoaded` | no | native | CLAUDE.md or equivalent rules file loaded/reloaded. |
| `ConfigChange` | no | native | Config file changes during a session. |
| `CwdChanged` | no | native | Working directory changes (e.g. via `cd`). |
| `PreCompact` | no | native | Before context compaction begins. |
| `PostCompact` | no | native | After context compaction completes. |
| `LspDiagnostics` | no | extension (opencode) | LSP diagnostics arrived. |

## file

| Event | Blocking | Origin | When |
|---|---|---|---|
| `FileChanged` | no | native | Watched file changes on disk. |

## worktree

| Event | Blocking | Origin | When |
|---|---|---|---|
| `WorktreeCreate` | no | native | Git worktree created. |
| `WorktreeRemove` | no | native | Git worktree removed. |

## mcp

| Event | Blocking | Origin | When |
|---|---|---|---|
| `Elicitation` | yes | native | MCP server requests user input. |
| `ElicitationResult` | no | native | User responds to MCP elicitation request. |

## memory (derived)

| Event | Blocking | Origin | When |
|---|---|---|---|
| `MemoryRead` | no | derived from `PreToolUse` | Read tool call targets a canonical memory path (CLAUDE.md, MEMORY.md, `.claude/memory/**`, `.agents/memory/`). |
| `MemoryWrite` | no | derived from `PreToolUse` | Write/Edit/NotebookEdit tool call targets the same paths. |

Both carry a `memory_kind` field: `project | user | skill | agent | unknown`.

## bus (kit-bus phase 1)

Item-lifecycle events for inbox containers. Canonical naming:
`<container>_<item>_<verb>`. The container-level pair
`inbox_created` / `inbox_destroyed` is RESERVED — separate ADR.

| Event | Blocking | Origin | When |
|---|---|---|---|
| `inbox_message_added` | no | native | A new entry arrived in an inbox container. |
| `inbox_message_updated` | no | native | An existing inbox entry was edited in-place. |
| `inbox_message_promoted` | no | native | An inbox entry was promoted to a downstream system (`tlc-task`, `ctxt-object`, `other`). |
| `inbox_message_dropped` | no | native | An inbox entry was removed without promotion (`user-deleted`, `ttl-expired`, `duplicate`). |

## Counting

- 26 native (sessions, prompt, tool, permission, notification, agent,
  task, context, file, worktree, mcp) + 4 bus.
- 4 extension (gemini/opencode): `BeforeToolSelection`,
  `LspDiagnostics`, `SessionDiff`, `SessionStatus`.
- 2 derived: `MemoryRead`, `MemoryWrite`.

Total: 36. Payload field definitions live alongside each entry in
`spec/events.yaml`; codegen depends on the file — never rename or
reorder without an ADR.
