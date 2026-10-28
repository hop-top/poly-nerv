# OpenCode — Hooks Research Dossier

> Richest native event surface (session + file + permission + LSP + TUI)
> but forces JS/TS plugin authorship. MCP tool calls don't fire hooks.

Last verified: 2026-04-11.

## Summary

- **Events:** 25+ native across session / tool / file / permission /
  LSP / TUI / message / server / shell
- **Config:** `opencode.json` + plugins in `.opencode/plugins/*.{js,ts}` or `~/.config/opencode/plugins/`
- **Format:** JavaScript/TypeScript plugin modules exporting hook handlers
- **Handler protocol:** in-process JS callbacks; throw to block `before` hooks
- **Matcher:** string or regex on tool name within handler body
- **Platform:** macOS + Linux + Windows
- **Maturity:** GA; richest lifecycle events but plugin-only model

## Sources

- Plugins docs: <https://opencode.ai/docs/plugins/>
- SDK reference: <https://deepwiki.com/sst/opencode/7-sdks-and-extension-points>
- Community guide: <https://gist.github.com/johnlindquist/0adf1032b4e84942f3e1050aba3c5e4a>
- YAML shim (community): <https://github.com/KristjanPikhof/OpenCode-Hooks>
- Claude-compat request: <https://github.com/anomalyco/opencode/issues/12472>
- MCP hook bug: <https://github.com/sst/opencode/issues/2319>

## Native events

### Session lifecycle

- `session.created` — session begins
- `session.idle` — session goes idle (maps to `Stop`)
- `session.updated` — session state updated
- `session.compacted` — context compacted (maps to `PostCompact`)
- `session.deleted` — session terminated
- `session.error` — session errored
- `session.diff` — session diff computed
- `session.status` — session status changed

### Tool lifecycle

- `tool.execute.before` — before tool call (can throw to block)
- `tool.execute.after` — after tool call

### File lifecycle

- `file.edited` — file was edited (unique to OpenCode)
- `file.watcher.updated` — file watcher detected change

### Permission lifecycle

- `permission.asked` — permission dialog shown
- `permission.replied` — user responded

### LSP

- `lsp.client.diagnostics` — LSP diagnostics arrived
- `lsp.updated` — LSP state updated

### TUI / commands

- `tui.prompt.append` — prompt appended in TUI
- `tui.command.execute` — slash command executing
- `tui.toast.show` — toast notification shown

### Messages

- `message.part.updated` — streaming message part updated
- `message.part.removed` — message part removed
- `message.removed` — full message removed
- `message.updated` — message updated

### Todo

- `todo.updated` — todo list updated

### Server / installation

- `server.connected` — MCP/LSP server connected
- `installation.updated` — installation state changed

### Shell

- `shell.env` — shell environment changed

### Experimental

- `experimental.session.compacting` — **before** compaction starts
  (maps to `PreCompact`; was previously thought impossible)

## Plugin schema (pseudocode)

```pseudocode
# .opencode/plugins/nerv-adapter.ts
export default (ctx) => ({
  "tool.execute.before": async (event) => {
    if (event.tool === "bash" && /^rm /.test(event.input.command)) {
      throw new Error("use `trash` per AGENTS.md")
    }
  },
  "session.created": async (event) => {
    // emit nerv SessionStart envelope
  },
  "file.edited": async (event) => { /* ... */ },
})
```

## Coverage vs Claude Code

| Claude event          | OpenCode status                            |
|-----------------------|---------------------------------------------|
| SessionStart          | ✅ session.created                          |
| UserPromptSubmit      | ≈ tui.prompt.append / message.part.updated |
| PreToolUse            | ✅ tool.execute.before                      |
| PermissionRequest     | ✅ permission.asked                         |
| PermissionDenied      | ✅ permission.replied (filter deny)         |
| PostToolUse           | ✅ tool.execute.after                       |
| PostToolUseFailure    | 🔧 tool.execute.after + error field         |
| Notification          | 🔧 session.error (partial)                  |
| SubagentStart         | 🔧 tool.execute.before match=task           |
| SubagentStop          | 🔧 tool.execute.after match=task            |
| TaskCreated           | 🔧 tool.execute.before match=todo_write     |
| TaskCompleted         | ✅ todo.updated filter                      |
| Stop                  | ✅ session.idle                             |
| StopFailure           | ✅ session.error                            |
| TeammateIdle          | ❌                                          |
| InstructionsLoaded    | 🔧 tool.execute.after read match            |
| ConfigChange          | 🔧 file.edited match                        |
| CwdChanged            | 🔧 bash regex                               |
| FileChanged           | ✅ file.edited                              |
| WorktreeCreate        | 🔧 bash regex                               |
| WorktreeRemove        | 🔧 bash regex                               |
| PreCompact            | ✅ experimental.session.compacting           |
| PostCompact           | ✅ session.compacted                        |
| Elicitation           | 🔧 tool.execute.before match=mcp__*         |
| ElicitationResult     | 🔧 tool.execute.after match=mcp__*          |
| SessionEnd            | ✅ session.deleted                          |

**Score: 25/26** (impossible: `TeammateIdle`; `PreCompact` now
available via experimental event)

## Unique events (no Claude equivalent)

- `lsp.client.diagnostics` — LSP diagnostics stream; nerv extension
  `lsp.diagnostics`
- `lsp.updated` — LSP state changes; nerv internal
- `session.diff` — session code diff computed; nerv extension
  `session.diff`
- `session.status` — generic status transitions; nerv internal
- `file.watcher.updated` — OS-level file watch; broader than
  `file.edited` (includes non-tool changes)
- `server.connected` — MCP/LSP server connection; nerv extension
  `server.connected`
- `shell.env` — shell env changes; nerv extension `shell.env`
- `tui.toast.show` — toast notifications; maps to `Notification`
- `permission.asked`/`replied` — OpenCode is the only CLI with
  first-class permission hooks. Claude's `PermissionRequest`/`Denied`
  are weaker because they fire only on denial decisions, not all
  requests.
- `experimental.session.compacting` — pre-compaction event OpenCode
  now has (experimental); Claude has `PreCompact`

## Strengths

- Richest permission lifecycle of any CLI.
- Native `file.edited` event (saves you from building a fswatch daemon).
- Both pre and post compaction events (`experimental.session.compacting`
  + `session.compacted`).
- LSP integration makes code-aware hooks possible (e.g., "block commit
  if diagnostics > threshold").
- `file.watcher.updated` gives OS-level file watching for free.
- `server.connected` enables MCP server lifecycle tracking.

## Known gotchas

- **JS/TS only** — forces plugin authors into one language. nerv adapter
  must bridge to shell/Python/Go handlers via IPC.
- **MCP tool calls don't fire plugin hooks** —
  <https://github.com/sst/opencode/issues/2319>. Workaround: MCP proxy.
- No `session.started` vs `session.resumed` distinction.
- `experimental.session.compacting` provides pre-compaction but is
  experimental — may change or break.
- Community requested Claude-Code-compat hook mode upstream:
  <https://github.com/anomalyco/opencode/issues/12472> (not yet merged).

## Integration notes for nerv

Adapter writes a single TypeScript plugin at `.opencode/plugins/nerv-adapter.ts`
that:

1. Spawns a long-lived Go subprocess (`nerv-adapter opencode`) on
   `session.created` via `child_process.spawn`.
2. Forwards every OpenCode event to the subprocess over stdin as JSON.
3. Reads decision JSON from the subprocess stdout.
4. Applies decisions: throw for block on `tool.execute.before`, mutate
   input for rewrite, emit synthetic events for matchers.

This keeps hook authors in any language via the normal nerv daemon, while
OpenCode sees only a single JS plugin.

Expected adapter size: ~400 LOC Go + ~150 LOC TypeScript shim.

## Open questions

1. Does `permission.replied` include the original request's tool context
   or just the decision?
2. Can plugins cancel a `session.compacted` retroactively? (Likely no;
   that's why `PreCompact` is impossible.)
3. Will MCP hook coverage land upstream? Track sst/opencode#2319.
4. Does `file.edited` debounce rapid saves or fire per-keystroke?
