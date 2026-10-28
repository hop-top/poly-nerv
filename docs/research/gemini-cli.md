# Gemini CLI — Hooks Research Dossier

> Closest drop-in analogue to Claude Code hooks. JSON config,
> regex matchers, stdin/stdout protocol, exit-code blocking.
> Best portability target after Claude.

Last verified: 2026-04-11.

## Summary

- **Events:** 11 native (tool + session + agent + model + compression)
- **Config:** `~/.gemini/settings.json` (global) or `.gemini/settings.json` (project)
- **Format:** JSON with `hooks` object keyed by event
- **Handler protocol:** executable; stdin JSON envelope; stdout JSON decision; exit code blocking
- **Matcher:** regex per-hook (all tools matchable)
- **Platform:** macOS + Linux + Windows
- **Maturity:** GA; recent release

## Sources

- Docs index: <https://geminicli.com/docs/hooks/>
- Events reference: <https://geminicli.com/docs/hooks/reference/>
- Writing hooks: <https://geminicli.com/docs/hooks/writing-hooks/>
- Repo docs: <https://github.com/google-gemini/gemini-cli/blob/main/docs/hooks/index.md>
- Announcement: <https://developers.googleblog.com/tailor-gemini-cli-to-your-workflow-with-hooks/>

## Native events

### Session lifecycle

- `SessionStart` — session begins
- `SessionEnd` — session ends

### Prompt lifecycle

- `BeforeModel` — before model invocation (closest to `UserPromptSubmit`)
- `AfterModel` — after model invocation

### Tool lifecycle

- `BeforeTool` — before any tool call
- `AfterTool` — after any tool call
- `BeforeToolSelection` — before model picks a tool (no Claude equivalent)

### Agent lifecycle

- `BeforeAgent` — fires after user prompt, before agent planning
- `AfterAgent` — fires after model generates final response per turn

### Context management

- `PreCompress` — before context compression

### Notifications

- `Notification` — CLI notification sent

## Config schema (pseudocode)

```pseudocode
{
  "hooks": {
    "BeforeTool": [
      {
        "matcher": "Bash|Shell",        # optional; regex for tools
        "sequential": false,            # serial vs parallel
        "hooks": [
          {
            "type": "command",          # only type supported
            "command": "path/to/handler",
            "name": "friendly-id",      # optional
            "timeout": 60000,           # ms; default 60000
            "description": "..."        # optional
          }
        ]
      }
    ],
    "SessionStart": [ ... ]
  }
}
```

## Handler contract

Same as Claude Code:

- stdin receives JSON envelope
- stdout optionally writes JSON decision
- exit code 0 = allow, 2 = block, other = warn

## Envelope (pseudocode)

```pseudocode
{
  "session_id": "...",
  "transcript_path": "...",
  "cwd": "...",
  "hook_event_name": "BeforeTool",
  "timestamp": "ISO 8601",
  "tool_name": "Shell",             # tool events only
  "tool_input": { ... }             # tool events only
}
```

## Decision (stdout JSON, optional)

```pseudocode
{
  "decision": "allow" | "deny" | "block",  # deny preferred over block
  "reason": "feedback message",
  "continue": false,                       # stops agent loop
  "stopReason": "...",
  "suppressOutput": false,
  "systemMessage": "warning text",
  "hookSpecificOutput": {
    "additionalContext": "...",
    "tool_input": { ... },                 # rewrite (BeforeTool)
    "tailToolCallRequest": { ... },        # append tool call
    "clearContext": false,                 # BeforeAgent/AfterAgent
    "toolConfig": {                        # BeforeToolSelection
      "mode": "...",
      "allowedFunctionNames": []
    }
  }
}
```

## Coverage vs Claude Code

| Claude event            | Gemini status                            |
|-------------------------|------------------------------------------|
| SessionStart            | ✅ SessionStart                           |
| UserPromptSubmit        | ≈ BeforeModel                             |
| PreToolUse              | ✅ BeforeTool                             |
| PermissionRequest       | 🔧 BeforeTool + auto-approve check        |
| PermissionDenied        | 🔧 BeforeTool exit=2                      |
| PostToolUse             | ✅ AfterTool                              |
| PostToolUseFailure      | 🔧 AfterTool + tool_response.error        |
| Notification            | ✅ Notification                           |
| SubagentStart           | ≈ BeforeAgent (per-turn, not subagent)    |
| SubagentStop            | ≈ AfterAgent (per-turn, not subagent)     |
| TaskCreated             | 🔧 BeforeTool match=TaskCreate            |
| TaskCompleted           | 🔧 AfterTool match=TaskUpdate + diff      |
| Stop                    | ≈ SessionEnd                              |
| StopFailure             | 🔧 SessionEnd + error marker              |
| TeammateIdle            | ❌                                        |
| InstructionsLoaded      | 🔧 AfterTool match=Read path~CLAUDE.md    |
| ConfigChange            | 🔧 AfterTool match=Write path~settings    |
| CwdChanged              | 🔧 AfterTool match=Bash regex `^cd`       |
| FileChanged             | 🔧 AfterTool match=Write\|Edit            |
| WorktreeCreate          | 🔧 AfterTool match=Bash regex             |
| WorktreeRemove          | 🔧 AfterTool match=Bash regex             |
| PreCompact              | ✅ PreCompress                            |
| PostCompact             | 🔧 AfterModel + compaction marker         |
| Elicitation             | 🔧 BeforeTool match=mcp__*                |
| ElicitationResult       | 🔧 AfterTool match=mcp__*                 |
| SessionEnd              | ✅ SessionEnd                             |

**Score: 25/26** (only `TeammateIdle` is impossible)

## Unique events (no Claude equivalent)

- `BeforeToolSelection` — fires before model picks which tool to call.
  Can rewrite tool availability via `toolConfig.allowedFunctionNames`.
  nerv extension: `tool.selection_pre`.
- `AfterModel` — post-LLM-response hook; fires per LLM response chunk.
  Access raw `llm_response` for logging/auditing. nerv extension:
  `prompt.post`.
- `BeforeAgent`/`AfterAgent` — per-turn agent lifecycle, not per-
  subagent. `clearContext` output can wipe context. Different from
  Claude's SubagentStart/Stop which track nested agents.

## Strengths

- Cleanest Claude-shaped config; near-identical schema structure.
- All tools matchable (no shell-only limitation like Codex).
- Built-in model-level hooks (`BeforeModel`, `AfterModel`) allow
  prompt-level guardrails.
- `BeforeToolSelection` is unique — no other CLI can rewrite tool
  availability per turn.
- `sequential` flag allows explicit serial vs parallel hook execution.
- `deny` decision preferred over `block` — cleaner semantics.

## Known gotchas

- `BeforeModel` vs `UserPromptSubmit` semantic drift — fires once per
  model call, which may be multiple times per user prompt if the turn
  includes multiple model roundtrips. nerv will map to `prompt.submit`
  with a `turn_index` field.
- `BeforeAgent`/`AfterAgent` are per-turn, not per-subagent — maps
  imperfectly to Claude's SubagentStart/Stop.
- `SessionEnd` fires on graceful exit only; process kill bypasses it.
- Only `command` hook type supported (no http/prompt/agent like Claude).
- Timeout in milliseconds (60000 default), not seconds like Claude.

## Integration notes for nerv

Adapter writes `~/.gemini/settings.json` entries pointing all events at
`nerv-adapter gemini <event>`. Synthesis required for ~12 Claude events
via matcher patterns. No binary wrapping needed.

Expected adapter size: ~350 LOC Go.

## Open questions

1. Is `BeforeModel` called once per user prompt or per model turn?
2. Does `AfterTool` envelope include `tool_response.error` field on
   failure, or does a separate event fire?
3. Can hooks register dynamic matchers (e.g., load from env)?
