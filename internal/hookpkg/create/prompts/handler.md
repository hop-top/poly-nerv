You are authoring a nerv hook handler from a natural-language intent.

# Intent

{{INTENT}}

# Constraints

- Language: {{LANG}} ("sh" = POSIX sh, no bashisms; "py" = python3 only).
- Handler must read ONE LINE of JSON (the envelope) on stdin and write
  ONE LINE of JSON (the decision) on stdout, then exit 0.
- Handler must be self-contained — no external scripts, no network calls
  unless the intent explicitly requires it.

# Envelope shape (stdin)

```json
{
  "nerv_version": "1.0.0-draft",
  "event": "PreToolUse",
  "event_id": "<uuid>",
  "timestamp": "2026-01-01T00:00:00Z",
  "cli":     {"name": "claude-code", "version": "1.0.0"},
  "session": {"id": "abc", "cwd": "/path"},
  "payload": { ... event-specific ... }
}
```

# Decision shape (stdout)

```json
{"action":"ALLOW|WARN|BLOCK|REWRITE","message":"<short reason>"}
```

# Available events

The list below is illustrative, NOT authoritative — the full catalog
(36 canonical entries: 26 native + 4 extension + 2 derived + 4 bus)
with semantic notes lives at `.agents/references/event-catalog.md`;
machine-readable ground truth is `spec/events.yaml`. If the intent
needs an event not listed here, consult the catalog before guessing.

Common events:

SessionStart, SessionEnd, UserPromptSubmit, PreToolUse, PostToolUse,
PostToolUseFailure, PermissionRequest, PermissionDenied, Notification,
SubagentStart, SubagentStop, TaskCreated, TaskCompleted, Stop,
StopFailure, TeammateIdle, InstructionsLoaded, ConfigChange, CwdChanged,
FileChanged, WorktreeCreate, WorktreeRemove, PreCompact, PostCompact,
Elicitation, ElicitationResult, BeforeToolSelection, LspDiagnostics,
SessionDiff, SessionStatus, MemoryRead, MemoryWrite, inbox_message_added,
inbox_message_updated, inbox_message_promoted, inbox_message_dropped.

# Output (REQUIRED)

Respond with STRICT JSON only — no prose, no markdown fences. Shape:

```json
{
  "name":      "<kebab-case-slug>",
  "events":    ["PreToolUse"],
  "script":    "<full handler source with shebang>",
  "rationale": "<one line>"
}
```

Rules:

- `name` is a short kebab-case slug derived from the intent (2-5 words).
- `events` is the inferred subscription. If the user supplied an explicit
  list via override, prefer that list verbatim:
  {{EVENTS_OVERRIDE}}
- `script` is the COMPLETE handler source, including shebang
  (`#!/bin/sh` or `#!/usr/bin/env python3`) and a top comment recording
  the intent.
- `rationale` is one line explaining why these events + this logic.

Begin response with `{` and end with `}`. No surrounding text.
