# Codex CLI — Hooks Research Dossier

> Feature-flagged Claude-Code-shaped hook schema. Scope is limited
> to `shell` + `apply_patch` tools. Requires layered wrapping for
> full nerv parity.

Last verified: 2026-04-11.

## Summary

- **Events:** 5 native event names (`SessionStart`, `UserPromptSubmit`,
  `PreToolUse`, `PostToolUse`, `Stop`); 3 fully equivalent to Claude,
  2 scope-limited (Bash only)
- **Scope:** PreToolUse/PostToolUse currently **Bash only**; doesn't
  intercept all shell calls yet. MCP, Write, WebSearch not hooked.
  `apply_patch` interception incomplete (`unified_exec` streaming).
- **Config:** `.codex/hooks.json` (repo) or `~/.codex/hooks.json`;
  multiple files merge (higher-precedence layers don't replace lower)
- **Feature flag:** `codex_hooks = true` in `~/.codex/config.toml`
  under `[features]`
- **Platform:** macOS + Linux. **Disabled on Windows.**
- **Format:** JSON, mirrors Claude Code schema
- **Maturity:** experimental; audit/logging-grade

## Sources

- Hooks docs: <https://developers.openai.com/codex/hooks>
- Advanced config: <https://developers.openai.com/codex/config-advanced>
- Plugins (separate system): <https://developers.openai.com/codex/plugins>
- Original RFC: <https://github.com/openai/codex/discussions/2150>
- Feature issue: <https://github.com/openai/codex/issues/2109>

## Codex's actual tool registry

Unlike Claude Code, Codex does not expose `Read`/`Write`/`Edit`/`WebFetch`
as distinct tools. The model gets a minimal surface and composes file
operations through shell commands. Inspecting `codex-rs/core/src/tools/`
in github.com/openai/codex reveals:

| Tool            | Purpose                                    | Hooked? |
|-----------------|--------------------------------------------|---------|
| `shell`         | execute shell command                      | ⚠️ partial |
| `local_shell`   | execute local shell (MCP-compatible alias) | ⚠️ partial |
| `apply_patch`   | structured patch/diff edits                | ⚠️ incomplete |
| `update_plan`   | internal plan/todo state                   | ❌       |
| `web_search`    | web search (when enabled)                  | ❌       |
| `mcp__<srv>__*` | MCP server tools                            | ❌       |

This is why "Bash only" is misleading — Codex routes most file I/O
through `shell`, so shell-hook matchers cover a surprising amount of
Claude-equivalent activity.

## Config schema

```pseudocode
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "shell|local_shell|apply_patch",
        "hooks": [
          {
            "type": "command",
            "command": "path/to/handler"
          }
        ]
      }
    ],
    "SessionStart": [ ... ]
  }
}
```

Handler fields also support `statusMessage` (UI feedback) and
`timeoutSec` (default 600s).

Plus `~/.codex/config.toml`:

```toml
[features]
codex_hooks = true
```

## Handler contract

Same Claude-shape: stdin JSON envelope, stdout JSON decision, exit code.

Common input fields: `session_id`, `transcript_path`, `cwd`,
`hook_event_name`, `model`. Turn-scoped hooks also get `turn_id`.

UserPromptSubmit and Stop don't support matchers (always fire).
Matcher `"*"` / `""` / omitted → match all occurrences.

## Native event coverage

| Claude event         | Codex status                              |
|----------------------|-------------------------------------------|
| SessionStart         | ✅ native                                  |
| UserPromptSubmit     | ✅ native                                  |
| PreToolUse           | ⚠️ Bash only; apply_patch incomplete       |
| PostToolUse          | ⚠️ Bash only; apply_patch incomplete       |
| Stop                 | ✅ native                                  |

Everything else requires workarounds (see below).

## Workaround strategy — 4 layers

### Layer 1 — shell regex dispatch

Hook `PreToolUse`/`PostToolUse` match `shell`, decode the command string,
emit synthetic events.

```pseudocode
cmd = input.tool_input.command
if cmd =~ /^cd /         → emit CwdChanged
if cmd =~ /git worktree add/ → emit WorktreeCreate
if cmd =~ /cat .*CLAUDE\.md/ → emit InstructionsLoaded
if cmd =~ /^rm /         → block; emit PermissionDenied
```

Covers: `CwdChanged`, `WorktreeCreate/Remove`, `InstructionsLoaded`,
`PermissionRequest/Denied` for shell ops, `FileChanged` partial,
`ConfigChange` partial.

### Layer 2 — background daemon

On `SessionStart`, spawn `fswatch` / `inotifywait` to cover things
shell matching misses.

```pseudocode
fswatch -0 $PWD --exclude '.git' | while read file; do
  emit FileChanged $file
done &
fswatch -0 ~/.codex/config.toml .codex/hooks.json | while read file; do
  emit ConfigChange $file
done &
```

Kill on `Stop` / `SessionEnd` via PID file.

Covers: `FileChanged` (comprehensive), `ConfigChange` (comprehensive),
`WorktreeCreate/Remove` (via git dir watches).

### Layer 3 — binary wrapper

Replace `codex` in `$PATH` with a shim that tees stdout/stderr and
parses Codex's output stream for events invisible to hooks.

```pseudocode
#!/usr/bin/env bash
# ~/.local/bin/codex
REAL=/opt/homebrew/bin/codex.real
exec > >(tee >(codex-event-extractor stdout))
exec 2> >(tee >(codex-event-extractor stderr) >&2)
emit SessionStart
"$REAL" "$@"
RC=$?
emit SessionEnd $RC
exit $RC
```

The extractor parses Codex's output (JSON mode if `--output-format json`,
else ANSI-stripped text) looking for:

- `[notify]` lines → `Notification`
- `compacting context` → `PreCompact` / `PostCompact`
- `plan updated` → `TaskCreated` / `TaskCompleted`

Covers: `Notification`, `PreCompact`/`PostCompact`, `TaskCreated`/`Completed`.

### Layer 4 — MCP proxy

For MCP `Elicitation`/`ElicitationResult` and any MCP tool call, insert
a proxy MCP server between Codex and the real servers.

```pseudocode
# mcp-hook-proxy
on_tool_call(req):
  emit PreToolUse {tool: req.name, input: req.params}
  if req.requires_user_input:
    emit Elicitation req
    resp = forward_to_real(req)
    emit ElicitationResult resp
  else:
    resp = forward_to_real(req)
  emit PostToolUse {tool: req.name, output: resp}
  return resp
```

Configure Codex to talk to `localhost:<port>` instead of real MCP servers.

Covers: MCP `Elicitation`, `ElicitationResult`, MCP tool `PreToolUse`/
`PostToolUse`.

## Coverage with layered workarounds

| Layer                         | Covers |
|-------------------------------|--------|
| Layer 0 (native, full parity) | 3      |
| Layer 1 (shell regex)         | + 9    |
| Layer 2 (fswatch daemon)      | + 3    |
| Layer 3 (binary wrapper)      | + 4    |
| Layer 4 (MCP proxy)           | + 4    |
| **Achievable total**          | **23/26** |

**Impossible:** `TeammateIdle` (no multi-agent model), `SubagentStart`/`Stop`
(no Task/subagent tool).

## Known gaps / issues

- `codex_hooks` is an opt-in flag under `[features]`, not default.
- No Windows support (Codex hooks explicitly disabled).
- `apply_patch` interception incomplete; `unified_exec` mechanism
  allows richer streaming but not all calls intercepted.
- "Doesn't intercept all shell calls yet, only the simple ones."
- MCP not hooked; requires Layer 4 for any MCP-aware guard.
- `update_plan` not hooked; todo tracking requires Layer 3 parsing.
- Multiple hooks.json files merge; higher-precedence layers don't
  replace lower-precedence hooks (additive merge).

## Integration notes for nerv

Codex is the hardest adapter. Expected layers required:

- **Minimum viable:** Layer 0 + Layer 1 (shell regex) → ~15 events covered
- **Full:** Layers 0-4 → 23 events covered

Expected adapter size: ~1200 LOC Go (daemon supervisor +
regex dispatcher + fswatch bridge + binary shim + MCP proxy).

## Open questions

1. Does `apply_patch` envelope include full patch text or just file paths?
2. Will `update_plan` hooking land upstream? Track
   <https://github.com/openai/codex/issues/2109>
3. Does `--output-format json` emit structured events for
   compaction/notifications, or only at turn boundaries?
4. Can MCP proxy insertion be automated per-session, or does it require
   manual `~/.codex/config.toml` editing?
