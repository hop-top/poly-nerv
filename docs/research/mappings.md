# Hooks Event Mappings — Claude Code ↔ Gemini ↔ Codex ↔ OpenCode

> Living document. Source of truth for nerv event taxonomy + per-CLI
> coverage. Update whenever a CLI ships hook changes. Feeds nerv
> adapter capability maps and `nerv doctor` output.

Last verified: 2026-04-08.

## Reference surface: Claude Code (26 events)

Claude Code has the richest hook surface. nerv's event taxonomy is a
superset of Claude's 26 events + OpenCode's file/permission extras.
Source: <https://code.claude.com/docs/en/hooks>

## Complete mapping table

| #  | Claude Code event       | Fires when                               | Gemini CLI               | Codex CLI                   | OpenCode                      |
|----|-------------------------|------------------------------------------|--------------------------|-----------------------------|-------------------------------|
| 1  | `SessionStart`          | session begin/resume                     | ✅ `SessionStart`         | ✅ `SessionStart`            | ✅ `session.created`           |
| 2  | `UserPromptSubmit`      | prompt submitted                         | ≈ `BeforeModel`          | ✅ `UserPromptSubmit`        | ≈ `tui.prompt.append`         |
| 3  | `PreToolUse`            | before tool call                         | ✅ `BeforeTool`           | ⚠️ `PreToolUse` (shell+apply_patch) | ✅ `tool.execute.before`  |
| 4  | `PermissionRequest`     | permission dialog appears                | 🔧 via `BeforeTool`       | 🔧 via shell denial          | ✅ `permission.asked`          |
| 5  | `PermissionDenied`      | auto-mode classifier denies              | 🔧 `BeforeTool` exit=2    | 🔧 shell block resp          | ✅ `permission.replied` (filter) |
| 6  | `PostToolUse`           | tool call succeeds                       | ✅ `AfterTool`            | ⚠️ `PostToolUse` (shell+apply_patch) | ✅ `tool.execute.after`  |
| 7  | `PostToolUseFailure`    | tool call fails                          | 🔧 `AfterTool`+status     | 🔧 `PostToolUse`+exit        | 🔧 `tool.execute.after`+error |
| 8  | `Notification`          | notification sent                        | ✅ `Notification`         | 🔧 binary wrapper            | 🔧 `session.error` (partial)  |
| 9  | `SubagentStart`         | subagent spawned                         | ✅ `BeforeAgent`          | ❌                           | 🔧 `tool.execute.before` match=task |
| 10 | `SubagentStop`          | subagent finishes                        | ✅ `AfterAgent`           | ❌                           | 🔧 `tool.execute.after` match=task |
| 11 | `TaskCreated`           | `TaskCreate` tool invoked                | 🔧 `BeforeTool` match     | 🔧 shell match for `tlc task create` | 🔧 `tool.execute.before` match=todo_write |
| 12 | `TaskCompleted`         | task marked completed                    | 🔧 `AfterTool` diff       | 🔧 shell match for `tlc task complete` | ✅ `todo.updated` filter |
| 13 | `Stop`                  | Claude finishes responding               | ≈ `SessionEnd`            | ✅ `Stop`                    | ✅ `session.idle`              |
| 14 | `StopFailure`           | turn ends with API error                 | 🔧 `SessionEnd`+err file  | 🔧 `Stop`+err field          | ✅ `session.error`             |
| 15 | `TeammateIdle`          | agent-team teammate going idle           | ❌                        | ❌                           | ❌                             |
| 16 | `InstructionsLoaded`    | CLAUDE.md / `.claude/rules/*.md` loaded  | 🔧 `AfterTool` Read match | 🔧 shell `cat .*CLAUDE\.md`  | 🔧 `tool.execute.after` read match |
| 17 | `ConfigChange`          | config file changes mid-session          | 🔧 `AfterTool` Write match| 🔧 fswatch daemon            | 🔧 `file.edited` match         |
| 18 | `CwdChanged`            | working directory changes (e.g. `cd`)    | 🔧 `AfterTool` Bash regex | 🔧 shell regex `^cd\b`       | 🔧 bash regex                  |
| 19 | `FileChanged`           | watched file changes on disk             | 🔧 `AfterTool` Write/Edit | 🔧 fswatch daemon            | ✅ `file.edited`               |
| 20 | `WorktreeCreate`        | worktree created (`--worktree` / isolation) | 🔧 Bash regex         | 🔧 shell regex `worktree add`| 🔧 bash regex                  |
| 21 | `WorktreeRemove`        | worktree removed                         | 🔧 Bash regex             | 🔧 shell regex `worktree remove` | 🔧 bash regex              |
| 22 | `PreCompact`            | before context compaction                | ✅ `PreCompress`          | ❌                           | ❌                             |
| 23 | `PostCompact`           | after compaction completes               | 🔧 `AfterModel` marker    | ❌                           | ✅ `session.compacted`         |
| 24 | `Elicitation`           | MCP server requests user input           | 🔧 `BeforeTool` mcp__*    | ❌ MCP proxy                 | 🔧 `tool.execute.before` match=mcp__* |
| 25 | `ElicitationResult`     | user responds to MCP elicitation         | 🔧 `AfterTool` mcp__*     | ❌ MCP proxy                 | 🔧 `tool.execute.after` match=mcp__* |
| 26 | `SessionEnd`            | session terminates                       | ✅ `SessionEnd`           | 🔧 `Stop` approx             | ✅ `session.deleted`           |

### Legend

- ✅ **native** — direct event, no plumbing, same semantics
- ≈ **close** — semantic mismatch but usable
- 🔧 **workaround** — synthesizable via matcher / regex / daemon / wrapper / MCP proxy
- ⚠️ **limited** — exists but scope too narrow (Codex: shell + apply_patch only)
- ❌ **impossible** — no feasible workaround with current CLI surface

## Coverage summary

| CLI         | Native ✅ | Close ≈ | Workaround 🔧 | Impossible ❌ | Achievable total |
|-------------|----------|---------|---------------|---------------|------------------|
| Claude Code | 26       | —       | —             | —             | 26/26            |
| Gemini CLI  | 10       | 3       | 12            | 1             | 25/26            |
| OpenCode    | 10       | 1       | 13            | 2             | 24/26            |
| Codex CLI   | 3        | —       | 12            | 11            | 15/26            |

Impossible events per CLI:

- **Gemini:** `TeammateIdle`
- **OpenCode:** `TeammateIdle`, `PreCompact`
- **Codex:** `TeammateIdle`, `SubagentStart/Stop`, `InstructionsLoaded`,
  `Notification`, `PreCompact`, `PostCompact`, `Elicitation`,
  `ElicitationResult`, `TaskCreated`, `TaskCompleted`

## Workaround patterns (cross-CLI)

### Pattern A — decode via matcher

Hook into a broad tool event and filter by tool name + argument regex.
Used for: `CwdChanged`, `WorktreeCreate`, `InstructionsLoaded`,
`TaskCreated`, `TaskCompleted`.

### Pattern B — background daemon

On session start, spawn `fswatch` / `inotifywait` and pipe events back
via named pipe or tail. Used for: `FileChanged`, `ConfigChange`. Required
for Codex because it has no file-watch event.

### Pattern C — status diff

Hook `tool.post`, compare tool_response to previous snapshot. Used for:
`PostToolUseFailure`, `TaskCompleted`, `StopFailure`.

### Pattern D — wrap the binary

Replace the CLI entry point with a shim that tees stdout/stderr, parses
the stream, and emits synthetic events. Used for: `Notification`,
`PreCompact`/`PostCompact` on CLIs without native events.

### Pattern E — MCP proxy

Insert an MCP server between the CLI and real MCP servers. Forwards all
traffic while emitting `tool.pre`/`tool.post`/`Elicitation*`. Required
for Codex MCP coverage.

## Codex: "shell + apply_patch only" explained

Codex CLI's `PreToolUse`/`PostToolUse` fire for the `shell` / `local_shell`
tool and (on recent builds) the `apply_patch` tool. This covers most
Codex activity because Codex exposes a minimal tool set to the model:

- `shell` / `local_shell` — all file reads/writes go through this
  (`cat`, `sed`, `rg`, `apply_patch <<EOF`)
- `apply_patch` — structured diff edits
- `update_plan` — internal plan/todo tool ❌ not hooked
- `web_search` — ❌ not hooked
- MCP tools — ❌ not hooked

So "Bash-only" is misleading: most Claude-equivalent `Read`/`Write`/`Edit`
activity IS observable via `shell` matchers. But `TaskCreated`,
`Notification`, MCP elicitations, and compaction remain unreachable
without binary wrapping or MCP proxying.

See `docs/research/codex-cli.md` for full tool registry details.

## Open questions

1. Does Gemini `BeforeModel` fire once per prompt or once per turn?
2. Does OpenCode's `session.idle` distinguish successful stop vs error stop?
3. Does Codex's `apply_patch` hook expose the full patch text in tool_input
   or only a reference?
4. Can Claude's `WorktreeCreate`/`WorktreeRemove` be triggered for hops
   created via `git hop add` (they're not created via `--worktree` flag)?

## Per-CLI research dossiers

See separate files in this directory:

- [claude-code.md](claude-code.md) — 26-event reference
- [gemini-cli.md](gemini-cli.md) — closest Claude clone
- [codex-cli.md](codex-cli.md) — shell+apply_patch reality
- [opencode.md](opencode.md) — richest native lifecycle, JS plugin model
- [kit-integration.md](kit-integration.md) — hop-top/kit delegation map
