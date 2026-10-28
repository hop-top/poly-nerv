# Goose CLI — nerv Adapter Research

## Overview

Goose (https://block.github.io/goose/) is a Rust-based AI agent by
Block. Binary: `goose`. Config: `~/.config/goose/config.yaml` or
`.goose/config.yaml`.

## Hook System

Goose has an **extension/plugin system** that supports stdio-based
extensions. Extensions can intercept and respond to events.

Additionally, Goose emits native SSE events for session lifecycle,
errors, notifications, permissions, subagent management, and task
execution.

### Integration Points

- Extensions configured in `config.yaml` under `extensions:` key
- Extensions communicate via stdio (JSON messages)
- Goose supports profiles for different extension configurations
- Native SSE stream from `goose-server` (reply.rs)
- Permission system (`crates/goose/src/permission/`)
- Subagent notifications (`notification_events.rs`)
- Task execution notifications (`TaskExecutionNotificationEvent`)

### Config Format

YAML file at `~/.config/goose/config.yaml` or `.goose/config.yaml`.

```yaml
extensions:
  nerv-bridge:
    type: stdio
    command: nerv
    args:
      - dispatch
      - goose
    enabled: true
```

## Event Mapping

### Native Events (11)

| nerv Event         | Goose Source        | Notes                          |
|-------------------|---------------------|--------------------------------|
| SessionStart      | session.start       | SSE session lifecycle          |
| SessionEnd        | session.end         | SSE session lifecycle          |
| Stop              | finish              | SSE Finish w/ reason + tokens  |
| Notification      | notification        | SSE ServerNotification         |
| PostCompact       | update_conversation | SSE UpdateConversation         |
| PermissionRequest | action_required     | ActionRequired route           |
| PermissionDenied  | permission_denied   | DenyOnce / AlwaysDeny          |
| SubagentStart     | subagent_created    | summon notification            |
| SubagentStop      | subagent_completed  | summon completed notification  |
| SubagentStop      | subagent_terminated | summon terminated notification |
| TaskCompleted     | tasks_complete      | TasksComplete notification     |
| StopFailure       | error               | SSE Error event                |

### Close Approximations (synthesized, from SSE/notifications)

| nerv Event        | Goose Source   | Source       | Notes                 |
|------------------|----------------|--------------|-----------------------|
| UserPromptSubmit | request.start  | sse          | Fires per request     |
| TaskCreated      | tasks_update   | notification | Pending->Running only |

### Synthesized Events (from nerv bridge extension)

| nerv Event          | Source      | Pattern                              |
|--------------------|-------------|--------------------------------------|
| PreToolUse         | extension   | tool.before from bridge              |
| PostToolUse        | extension   | tool.after from bridge               |
| PostToolUseFailure | extension   | tool.error from bridge               |
| FileChanged        | extension   | developer ext file ops               |
| CwdChanged         | shell-match | developer__shell `cd` command        |
| WorktreeCreate     | shell-match | worktree add / git hop add           |
| WorktreeRemove     | shell-match | worktree remove / git hop remove     |
| PreCompact         | signal      | compaction request from session      |
| InstructionsLoaded | extension   | skills extension load event          |

### Unsupported Events

TeammateIdle, ConfigChange, Elicitation, ElicitationResult,
BeforeToolSelection, LspDiagnostics, SessionDiff, SessionStatus.

## Detection

- Binary: `goose --version` or `goose version`
- Config: check `~/.config/goose/config.yaml` or `.goose/config.yaml`

## References

- https://block.github.io/goose/
- https://block.github.io/goose/docs/getting-started/installation
- https://block.github.io/goose/docs/getting-started/using-extensions
