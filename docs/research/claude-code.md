# Claude Code — Hooks Research Dossier

> Reference CLI for nerv. Richest native hook surface. nerv event
> taxonomy is a superset of Claude's 26 events.

Last verified: 2026-04-11. Source: <https://code.claude.com/docs/en/hooks>

## Summary

- **Events:** 26 (see table)
- **Config:** `~/.claude/settings.json` (global) or project `.claude/settings.json`
- **Format:** JSON with `hooks` object keyed by event name
- **Handler types:** `command` (exec), `http` (webhook), `prompt`
  (inline LLM), `agent` (subagent)
- **Handler protocol:** stdin JSON envelope; stdout JSON decision;
  exit codes signal allow/warn/block
- **Matcher:** per-hook `matcher` — exact string, `|`-separated list,
  or JS regex (auto-detected); matched field varies per event
- **Platform:** macOS + Linux + Windows (via WSL)
- **Blocking:** exit code 2 = block (event-dependent); exit 0 = allow
- **Async:** `"async": true` runs hook in background without blocking
- **Kill switch:** `"disableAllHooks": true` at any config level

## The 26 events

1. `SessionStart` — session begins or resumes
2. `UserPromptSubmit` — user submits prompt, before Claude processes
3. `PreToolUse` — before any tool call executes; can block
4. `PermissionRequest` — permission dialog appears
5. `PermissionDenied` — tool call denied by auto-mode classifier
6. `PostToolUse` — after tool call succeeds
7. `PostToolUseFailure` — after tool call fails
8. `Notification` — Claude Code sends a notification
9. `SubagentStart` — subagent spawned
10. `SubagentStop` — subagent finishes
11. `TaskCreated` — task created via `TaskCreate`
12. `TaskCompleted` — task marked completed
13. `Stop` — Claude finishes responding
14. `StopFailure` — turn ends due to API error
15. `TeammateIdle` — agent-team teammate going idle
16. `InstructionsLoaded` — CLAUDE.md or `.claude/rules/*.md` loaded
17. `ConfigChange` — config file changes during session
18. `CwdChanged` — working directory changes
19. `FileChanged` — watched file changes on disk
20. `WorktreeCreate` — worktree created
21. `WorktreeRemove` — worktree removed
22. `PreCompact` — before context compaction
23. `PostCompact` — after compaction completes
24. `Elicitation` — MCP server requests user input
25. `ElicitationResult` — user responds to MCP elicitation
26. `SessionEnd` — session terminates

## Config schema (pseudocode)

```pseudocode
{
  "hooks": {
    "<EventName>": [
      {
        "matcher": "<matcher>",           # optional; field varies per event
        "sequential": false,              # optional; serial vs parallel
        "hooks": [
          {
            "type": "command",            # or "http", "prompt", "agent"
            "command": "<shell-snippet>", # command type
            "url": "<endpoint>",          # http type
            "headers": {},                # http type
            "allowedEnvVars": [],         # http type (env → header expansion)
            "prompt": "<text>",           # prompt/agent type
            "model": "<model-name>",      # prompt/agent type
            "if": "<permission-rule>",    # conditional guard
            "timeout": <seconds>,         # optional; default 600
            "statusMessage": "<text>",    # optional; spinner text
            "once": false,               # fire only once per session
            "async": false,              # background execution
            "shell": "bash"              # shell override
          }
        ]
      }
    ]
  },
  "disableAllHooks": false               # kill switch
}
```

Config locations (priority order):
1. Managed policy settings (admin-controlled)
2. `~/.claude/settings.json` (user global)
3. `.claude/settings.json` (project, shareable)
4. `.claude/settings.local.json` (project, local only)
5. Plugin `hooks/hooks.json`
6. Skill/agent frontmatter hooks

## Envelope (stdin JSON)

Claude writes a JSON object to the hook's stdin. Fields vary per
event but the common shell is:

```pseudocode
{
  "session_id": "...",
  "transcript_path": "...",
  "cwd": "...",
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash",
  "tool_input": { ... },
  "tool_response": { ... },      # PostToolUse only
  "message": "..."                # Notification only
}
```

## Decision (stdout JSON, optional)

Multiple patterns depending on event:

```pseudocode
# Pattern 1 — common (UserPromptSubmit, PostToolUse, Stop, etc.)
{
  "decision": "allow" | "block",
  "reason": "human-readable message",
  "continue": true | false,
  "stopReason": "...",
  "suppressOutput": false,
  "systemMessage": "warning shown to user"
}

# Pattern 2 — PreToolUse (rich permission control)
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow|deny|ask|defer",
    "permissionDecisionReason": "...",
    "updatedInput": { ... },        # rewrite tool input
    "additionalContext": "..."
  }
}

# Pattern 3 — PermissionRequest (allow/deny + permission updates)
{
  "hookSpecificOutput": {
    "hookEventName": "PermissionRequest",
    "decision": {
      "behavior": "allow|deny",
      "updatedInput": { ... },
      "updatedPermissions": [ ... ],
      "message": "...",
      "interrupt": false
    }
  }
}
```

Output capped at 10,000 chars; overflow saved to file with preview.

## Exit codes

| Code | Meaning                                    |
|------|--------------------------------------------|
| 0    | success; stdout parsed as JSON output      |
| 2    | block (event-dependent; see below)         |
| other| non-blocking warning; stderr logged        |

Exit 2 **can block:** PreToolUse, PermissionRequest,
UserPromptSubmit, Stop, SubagentStop, TeammateIdle, TaskCreated,
TaskCompleted, ConfigChange, Elicitation, ElicitationResult,
WorktreeCreate.

Exit 2 **cannot block** (already happened): PostToolUse,
PostToolUseFailure, PermissionDenied, Notification, SubagentStart,
SessionStart, SessionEnd, CwdChanged, FileChanged, PreCompact,
PostCompact, InstructionsLoaded, WorktreeRemove.

**WorktreeCreate special case:** any non-zero exit aborts creation
(not just exit 2).

## Matcher field varies by event

| Events                                          | Matched field          |
|-------------------------------------------------|------------------------|
| PreToolUse, PostToolUse, PostToolUseFailure,    | `tool_name`            |
|   PermissionRequest, PermissionDenied           |                        |
| SessionStart                                    | `source`               |
| SessionEnd                                      | `reason`               |
| Notification                                    | `notification_type`    |
| SubagentStart, SubagentStop                     | `agent_type`           |
| PreCompact, PostCompact                         | compaction trigger     |
| ConfigChange                                    | config source          |
| StopFailure                                     | error type             |
| InstructionsLoaded                              | load reason            |
| Elicitation, ElicitationResult                  | MCP server name        |
| UserPromptSubmit, Stop, TeammateIdle,           | N/A (always fires)     |
|   TaskCreated, TaskCompleted, WorktreeCreate,   |                        |
|   WorktreeRemove, CwdChanged                    |                        |

Matcher evaluation: `"*"` / `""` / omitted → match all;
`[a-zA-Z0-9_|]` only → exact or `|`-list; other chars → JS regex.

## Strengths

- Only CLI to cover workspace lifecycle (`CwdChanged`,
  `WorktreeCreate`, `InstructionsLoaded`, `ConfigChange`).
- Only CLI with MCP elicitation hooks.
- Only CLI with distinct success/failure variants
  (`PostToolUseFailure`, `StopFailure`).
- Only CLI with multi-agent team awareness (`TeammateIdle`).
- Only CLI with 4 handler types (command, http, prompt, agent).
- Matcher regex works across all tools (not scoped to one family).

## Known gotchas

- Hooks run in a subprocess per event; no persistent state between
  events unless you manage it externally (file, pipe, daemon).
- `timeout` default 600s per-hook; long hooks block the tool call.
- `PreCompact` runs before compaction but cannot cancel it.
- `InstructionsLoaded` fires on every reload, not just initial load;
  matcher can filter by load reason (session_start, compact, etc.).
- Exit code 2 blocking is event-dependent (see table above).
- `WorktreeCreate` any non-zero exit aborts (stricter than other events).
- Hook stdout must be pure JSON; shell profile output interferes.
- `"async": true` hooks run in background — no decision possible.

## Open questions

1. Does `WorktreeCreate` fire for `git hop add` or only `--worktree` flag?
2. Does `TaskCompleted` fire synchronously before or after the Task
   tool returns control?
3. What's the envelope shape for `Elicitation` — does it include the
   MCP server's full request JSON-RPC payload?

## Integration notes for nerv

Claude Code is the **reference adapter**. nerv passes native events
through 1:1 with minimal translation. The adapter only needs to:

1. Write a single `hooks.yaml` entry per event, pointing all 26 events
   at `nerv-adapter claude <event-name>`.
2. Translate Claude's envelope JSON to nerv envelope (rename fields,
   add `cli.name=claude`, `synthesized=false`).
3. Translate nerv decision JSON back to Claude decision JSON + exit code.

No synthesis required. Expected adapter size: ~200 LOC Go.
