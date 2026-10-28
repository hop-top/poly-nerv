# OpenHands — Hooks Research Dossier

> Python-based autonomous AI agent (formerly OpenDevin). Internal
> event stream architecture provides richer hook surface than most
> newer tools. JSON config, PascalCase event names.

Last verified: 2026-04-17.

## Summary

- **Events:** 6 native (session + tool lifecycle + stop)
- **Config:** `~/.openhands/hooks.json` (global) or `.openhands/hooks.json`
  (project)
- **Format:** JSON with `hooks` object keyed by event name
- **Handler protocol:** executable command; stdin JSON; stdout JSON decision
- **Binary:** `openhands`
- **Detection:** `openhands --version`
- **Platform:** macOS + Linux (Docker-based)
- **Maturity:** Active development
- **Repo:** <https://github.com/All-Hands-AI/OpenHands>

## Native events

### Session lifecycle

- `SessionStart` — session begins
- `SessionEnd` — session ends

### Tool lifecycle

- `PreToolUse` — before tool execution; handler can block
- `PostToolUse` — after tool execution completes

### Other

- `UserPromptSubmit` — user submits a prompt
- `Stop` — agent response ends

## Synthesized events

From internal event stream (Actions/Observations):

- `SubagentStart` — from AgentDelegateAction
- `SubagentStop` — from AgentDelegateObservation
- `TaskCreated` — from TaskTrackingAction
- `TaskCompleted` — from TaskTrackingObservation
- `FileChanged` — from FileWrite/EditObservation
- `CwdChanged` — from CmdRunAction with `cd` command
- `PreCompact` — from CondensationAction
- `PostCompact` — from CondensationObservation
- `Elicitation` — from MCPAction
- `ElicitationResult` — from MCPObservation
- `PostToolUseFailure` — from ErrorObservation (tool-related)
- `StopFailure` — from ErrorObservation (agent/session error)
- `InstructionsLoaded` — from RecallObservation
- `PermissionDenied` — from UserRejectObservation

## Unsupported events

TeammateIdle, Notification, ConfigChange, WorktreeCreate,
WorktreeRemove, BeforeToolSelection.

## Config format

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "nervd",
            "timeout": 30000
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "nervd",
            "timeout": 30000
          }
        ]
      }
    ]
  }
}
```

## Decision format

```json
{
  "action": "allow|block",
  "reason": "optional message"
}
```

Exit codes: 0 = allow, 2 = block.

## Notes

- OpenHands runs primarily via Docker; hook commands execute in
  the container context
- Uses Claude Code-compatible hooks system (PR #1467)
- `CmdRunAction` is OpenHands' native shell execution action type
