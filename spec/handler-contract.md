# Handler Contract Specification

> Defines the protocol between nerv dispatcher and hook handlers.
> Single execution mode: exec (fork process per event).

## Event Origin

Every event in `spec/events.yaml` declares an `origin` field:

| Origin      | Source                                              |
|-------------|-----------------------------------------------------|
| `native`    | Emitted by an adapter from the host CLI's lifecycle |
| `extension` | Native to a subset of CLIs (e.g. Gemini-only)       |
| `derived`   | Synthesized by the dispatcher from a native event   |

Adapter `capabilities.yaml` (see `capabilities.schema.json`)
declares which events the adapter emits natively vs. synthesizes.
The catalog `origin` field is the consumer-side counterpart:
handlers can branch on origin to decide whether to expect the event
on every CLI (`native`), only some (`extension`), or as a
dispatcher-emitted derivative (`derived`).

Derived events are emitted in addition to (not instead of) their
source event so existing native consumers keep working. Derivation
rules live in the `derivation:` block of each derived event entry.

## Protocol Overview

Handler receives JSON envelope on stdin (one object, no streaming).
Handler writes JSON decision to stdout (one object).
Exit code signals intent. Handler MUST exit after writing stdout.

## Exit Codes

| Code | Meaning  | Behavior                                   |
|------|----------|--------------------------------------------|
| 0    | allow    | Event proceeds unchanged                   |
| 1    | warn     | Event proceeds; message shown to user      |
| 2    | block    | Event rejected; message shown to user      |
| 3    | rewrite  | Event proceeds with modified payload       |
| >=4  | error    | Fail-open: log error, allow event          |

## Envelope (stdin)

Dispatcher writes a single JSON object to handler's stdin then
closes the write end. The object is the full envelope
(`envelope.schema.json`); event-specific fields live under `payload`
(see `events.yaml`):

```json
{
  "nerv_version": "1.0.0-draft",
  "event": "PreToolUse",
  "event_id": "c7d2f1aa-9c4d-4b2e-9b1a-1a2b3c4d5e6f",
  "timestamp": "2026-04-11T12:00:00Z",
  "cli": { "name": "claude-code", "version": "2.1.0" },
  "session": { "id": "abc-123", "cwd": "/home/dev/project" },
  "payload": {
    "tool_name": "Bash",
    "tool_input": { "command": "rm -rf /" },
    "session_id": "abc-123"
  }
}
```

A shell handler reads the command with
`jq -r '.payload.tool_input.command // empty'`.

Envelope max size: 1 MB. Envelopes exceeding this limit are
rejected before reaching any handler (see failure-modes.md #7).

## Decision (stdout)

Handler writes a single JSON object to stdout. Empty stdout is
treated as `{"action": "allow"}`.

```yaml
action: allow | warn | block | rewrite   # required
message: string                          # optional; shown for warn/block
rewrite: object                          # optional; replacement payload (action=rewrite)
metadata: object                         # optional; attached to event log entry
```

Decision schema:

| Field      | Type   | Required | Notes                                  |
|------------|--------|----------|----------------------------------------|
| `action`   | string | yes      | One of: allow, warn, block, rewrite    |
| `message`  | string | no       | Human-readable; shown for warn/block. Accumulated alongside rewrite if both present. |
| `rewrite`  | object | no       | Replacement payload; action must be rewrite |
| `metadata` | object | no       | Arbitrary KV; persisted in event log   |

Action MUST match exit code:
- `allow` -> exit 0
- `warn` -> exit 1
- `block` -> exit 2
- `rewrite` -> exit 3

Mismatch: exit code wins. Dispatcher logs the mismatch at
level=warn. Fields are interpreted against the effective action:

- exit 0 (allow): ignore `message` and `rewrite`; honor `metadata`
- exit 1 (warn): honor `message`; ignore `rewrite`; honor `metadata`
- exit 2 (block): honor `message`; ignore `rewrite`; honor `metadata`
- exit 3 (rewrite): honor `rewrite`; ignore `message`; honor `metadata`

## Merge Semantics

When multiple hooks fire for the same event, decisions merge:

**Priority:** block > rewrite (compose) > warn (accumulate) > allow

Rules:
1. First `block` wins; remaining hooks skipped
2. Multiple `rewrite` decisions compose left-to-right (sequential
   application; later rewrites see output of earlier ones)
3. Multiple `warn` messages concatenate (newline-separated)
4. If a hook exits >=4, treat that hook as error: log it and
   continue merge as if that hook returned `allow`. This fail-open
   behavior applies only to the errored hook — it does not override
   `block`, `rewrite`, or `warn` decisions from other hooks.

Composition example (3 hooks on `PreToolUse`):

```
Hook A: rewrite { tool_input: { command: "ls" } }
Hook B: warn "Suspicious command"
Hook C: rewrite { tool_input: { command: "ls -la" } }
```

Result: rewrite wins (composed A then C), warn accumulated.
Final: `{ action: "rewrite", rewrite: { tool_input: { command: "ls -la" } }, message: "Suspicious command" }`

## Timeout

| Setting         | Value      |
|-----------------|------------|
| Default         | 30 seconds |
| Configurable    | Per-hook in `hooks.yaml` (`timeout` field)  |
| On timeout      | Kill process, treat as error -> fail-open   |
| Minimum allowed | 1 second   |
| Maximum allowed | 300 seconds (5 minutes)                     |

Timeout covers wall-clock time from fork to exit. Includes handler
startup, processing, and stdout write.

## Environment Variables

Dispatcher sets these before forking the handler process:

| Variable          | Value                          | Example                |
|-------------------|--------------------------------|------------------------|
| `NERV_EVENT`      | Event name                     | `PreToolUse`           |
| `NERV_CLI`        | CLI name                       | `claude`               |
| `NERV_SESSION_ID` | Session identifier             | `abc-123`              |
| `NERV_CWD`        | Current working directory      | `/home/dev/project`    |
| `NERV_HOOK_NAME`  | Name of hook being executed    | `block-rm-rf`          |

These supplement the stdin envelope. Handlers MAY use either source.
Environment variables exist for handlers that cannot parse JSON
(e.g., simple shell scripts).

## Execution Model

- **Mode:** exec only (fork + exec per event)
- **Concurrency:** hooks for a single event run sequentially
  (ordering defined by `hooks.yaml` array position)
- **Isolation:** each handler gets its own process; no shared memory
- **Working directory:** set to `NERV_CWD`
- **Stdin:** JSON envelope; closed after write
- **Stdout:** JSON decision; read until EOF or timeout
- **Stderr:** always captured; not parsed; logged at `debug` by
  default, escalated to `error` on exit >=4, timeout, or parse failure

## Constraints

- Handler MUST be executable (`chmod +x`)
- Handler MUST NOT read from TTY (stdin is a pipe)
- Handler SHOULD exit quickly; long-running work deferred to async
- Handler MUST NOT assume persistent state between invocations
- Handler MAY write to filesystem for cross-invocation state
  (use `flock` for concurrency safety; see failure-modes.md #12)
