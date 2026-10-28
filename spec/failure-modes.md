# Failure Mode Matrix

> Nerv MUST NEVER block a developer's workflow due to its own failures.

This principle governs every failure path. Nerv exists to protect
developers from risky tool calls — not to become a risk itself.

## Failure Categories

### Fail-Open

Default category. On failure, log the error and allow the event to
proceed. Developer workflow continues uninterrupted. Used when the
failure is in nerv's own machinery (hook execution, config parsing,
output validation).

### Fail-Safe

Used when allowing a malformed or oversized input could cause
downstream harm. The envelope is rejected or the operation is
blocked, but with a clear error returned to the adapter. The
adapter decides how to retry or degrade.

### Fail-Forward

Used when a preferred path is unavailable but an alternative
exists. Nerv transparently falls back to the alternative path
without user intervention.

## Matrix

| #  | Codepath | Category | Expected behavior | Recovery |
|----|----------|----------|-------------------|----------|
| 1  | Hook process exits >=4 | fail-open | Log error, allow event | Auto |
| 2  | Hook process times out | fail-open | Kill process, log, allow | Auto |
| 3  | Hook stdout is invalid JSON | fail-open | Log parse error, allow | Auto |
| 4  | Hook stdout is valid JSON but wrong schema | fail-open | Log validation error, allow | Auto |
| 5  | hooks.yaml parse error | fail-open | Log, skip bad layer, use remaining layers | Auto |
| 6  | hooks.yaml missing entirely | fail-open | No hooks active, log info | Auto |
| 7  | Envelope too large (>1MB) | fail-safe | Reject envelope, return error to adapter | Adapter retries with smaller payload |
| 8  | Socket connection refused (daemon mode) | fail-forward | Fall back to stateless dispatch | Auto |
| 9  | Event log write failure | fail-open | Log error, continue dispatch (events lost) | Manual: check disk/permissions |
| 10 | Adapter detection fails | fail-open | Log, skip adapter, continue | Manual: check CLI installation |
| 11 | Redaction exceeded time budget | fail-safe | Timeout redaction (100ms), reject envelope | Audit patterns + input size |
| 12 | Concurrent hook writes to same file | fail-safe | flock prevents; second hook waits or times out | Auto via lock timeout |

## Detail: Fail-Open Entries

**#1 — Hook process exits >=4**
Handler returned an unexpected exit code. Could be a crash, missing
dependency, or bug in handler code. Nerv logs the exit code and
stderr (if any) at level=error, then returns `allow` to the
adapter. No user-visible interruption.

**#2 — Hook process times out**
Handler exceeded its configured timeout (default 30s). Nerv sends
SIGTERM, waits 5s, then SIGKILL. Logs timeout at level=error.
Returns `allow`. Common cause: handler waiting on network or
stuck in a loop.

**#3 — Hook stdout is invalid JSON**
Handler wrote non-JSON to stdout (e.g., a stack trace or binary
data). Note: empty stdout is NOT invalid — it is treated as
`{"action": "allow"}` per handler-contract.md. Nerv logs non-empty
invalid output (truncated to 1KB) at level=error. Returns `allow`.

**#4 — Hook stdout is valid JSON but wrong schema**
Handler wrote valid JSON but missing `action` field or unknown
action value. Nerv logs the JSON and validation error at
level=error. Returns `allow`.

**#5 — hooks.yaml parse error**
One config layer has invalid YAML. Nerv logs the path and parse
error, skips that layer, merges remaining layers. Partial config
is better than no config. `nerv doctor` reports the broken layer.

**#6 — hooks.yaml missing entirely**
No hooks.yaml found at any layer. Nerv logs at level=info (not
error — this is a valid state). No hooks fire; all events pass
through. `nerv doctor` reports "no hooks configured".

**#9 — Event log write failure**
SQLite write fails (disk full, permissions, corruption). Nerv logs
the write error at level=error via stderr. Dispatch continues;
events are lost for this period. `nerv doctor` checks event log
health.

**#10 — Adapter detection fails**
CLI binary not found, version check fails, or adapter init errors.
Nerv logs at level=error, skips the adapter, continues with other
adapters. Manual fix: install/update the CLI, run `nerv doctor`.

## Detail: Fail-Safe Entries

**#7 — Envelope too large (>1MB)**
Adapter sent an envelope exceeding the 1MB limit. Nerv rejects
before any handler sees it. Returns structured error to adapter
with `code: ENVELOPE_TOO_LARGE`. Adapter MAY retry with truncated
tool_input/tool_response fields.

**#11 — Redaction exceeded time budget**
Redaction processing exceeded its 100ms budget (e.g., expensive
pattern evaluation or unusually large input). Go's `regexp` (RE2)
is immune to catastrophic backtracking, but large inputs can still
exceed the budget. Nerv aborts redaction and rejects the envelope
(not sent to handlers with un-redacted secrets). Returns error to
adapter. Fix: audit redaction patterns and input size in config.

**#12 — Concurrent hook writes to same file**
Two hooks write to the same state file (e.g., a shared JSON
counter). Nerv handlers SHOULD use `flock` advisory locks.
Second writer waits up to lock timeout (default 5s). If timeout
expires, second write fails; handler exits with error (fail-open
at handler level per #1).

## Detail: Fail-Forward Entries

**#8 — Socket connection refused (daemon mode)**
nervd daemon is not running or socket is stale. Adapter falls back
to stateless dispatch: fork handler directly without going through
the daemon. Slower (no connection reuse, no cached matchers) but
functionally identical. Logged at level=warn on first occurrence,
then suppressed for 60s to avoid log spam.

## Observability

All failure modes emit structured log entries via `kit/log`:

```json
{
  "level": "error",
  "msg": "hook timeout",
  "hook": "block-rm-rf",
  "event": "PreToolUse",
  "cli": "claude",
  "timeout_s": 30,
  "pid": 12345
}
```

`nerv doctor` aggregates recent failures from the event log and
reports health status per failure category.
