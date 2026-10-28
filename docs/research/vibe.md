# Vibe CLI (Mistral) — Hooks Research Dossier

> Mistral's AI coding CLI. Newer tool with emerging hook support.
> TOML config, event-driven handlers, synthesis-heavy adapter.

Last verified: 2026-04-17.

## Summary

- **Events:** 6 native (tool + compact + config lifecycle)
- **Config:** `~/.vibe/config.toml` (global) or `.vibe/config.toml` (project)
- **Format:** TOML with `[hooks]` section and `[[hooks.handlers]]` arrays
- **Handler protocol:** executable command; stdin JSON; stdout JSON decision
- **Binary:** `vibe`
- **Detection:** `vibe --version`
- **Platform:** macOS + Linux
- **Maturity:** Early/emerging

## Native events

### Tool lifecycle

- `ToolCallEvent` — before tool execution; handler can block
- `ToolResultEvent` — after tool execution completes

### Compact lifecycle

- `CompactStartEvent` — context compaction begins
- `CompactEndEvent` — context compaction ends

### Other

- `UserMessageEvent` — user submits a prompt
- `AgentProfileChangedEvent` — agent profile/config changed

## Close-mapped events

- `AssistantEvent` — maps to Stop when response ends
- `WaitingForInputEvent` — maps to Elicitation

## Synthesized events

Via middleware callbacks:

- `PermissionRequest` — from ApprovalCallback
- `PermissionDenied` — from ApprovalCallback (rejected)
- `SubagentStart` — from SwitchAgentCallback
- `ElicitationResult` — from UserInputCallback
- `SessionStart` — from middleware before_turn() on first turn
- `SessionEnd` — from middleware reset() or session teardown

## Unsupported events

SubagentStop, TaskCreated, TaskCompleted, StopFailure,
TeammateIdle, Notification, InstructionsLoaded, CwdChanged,
FileChanged, WorktreeCreate, WorktreeRemove, PostToolUseFailure,
BeforeToolSelection.

## Config format

```toml
[hooks]
enabled = true

[[hooks.handlers]]
event = "ToolCallEvent"
type = "command"
command = "nervd"
timeout = 30000

[[hooks.handlers]]
event = "UserMessageEvent"
type = "command"
command = "nervd"
timeout = 30000
```

## Decision format

```json
{
  "decision": "allow|block",
  "reason": "optional message"
}
```

Exit codes: 0 = allow, 2 = block.
