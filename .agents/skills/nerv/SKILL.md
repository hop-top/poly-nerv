---
name: nerv
description: "Universal Hooks Protocol — write hooks once, run them on every AI coding CLI. Use when the user wants to install, author, test, replay, disable, or debug hooks across Claude Code, Codex, Gemini, OpenCode, OpenHands, Aider, Goose, or Vibe."
license: MIT
metadata:
  status: stable
  version: "0.1"
---

# Universal Hooks Protocol — nerv

You build a guardrail once. nerv runs it on every AI coding CLI the
user touches.

The problem this skill solves: every AI CLI ships its own hook
surface — different config formats, different event names, different
blocking semantics. Maintaining N copies of the same `block-rm-rf`
hook for N CLIs is unsustainable. nerv defines one wire format
(envelopes), one taxonomy (36 canonical events), and one handler
contract (exit codes 0–3); per-CLI adapters translate everything
back and forth so your hook code never changes.

The CLI is one binary (`nerv`) plus a daemon (`nervd`) plus five
client SDKs (Go, TS, Python, Rust, PHP). This plugin surfaces nerv
to agents as a single skill (this file) and one slash command per
shipped user-facing verb. Each verb section below carries a
**Status** line.

## Use this skill when

- The user wants to add, remove, list, or audit guardrail hooks for
  an AI coding CLI.
- The user wants the same hook to run on multiple CLIs without
  duplicating the script (cross-CLI portability is the whole point).
- The user wants to test a hook against a synthetic event payload
  before letting it run on real CLI traffic.
- The user wants to replay a recorded event through the current hook
  pipeline (forensics, debugging, regression checking).
- The user wants to silence or scope down a hook without deleting it
  (disable a whole hook, an event class, or a specific binding).
- The user wants to install or refresh per-CLI adapters
  (`claude`, `codex`, `gemini`, `opencode`, `openhands`, `aider`,
  `goose`, `vibe`).
- The user wants a health check of the dispatch pipeline (`doctor`).
- The user mentions: hook, guardrail, audit log, block command,
  rewrite payload, fail-open, envelope, PreToolUse / PostToolUse /
  SessionStart, hooks.yaml, `.nerv/`, nervd, adapter, replay,
  doctor, dispatch, marketplace, ctxt capture.

## How nerv thinks about events (envelope → decision)

A **hook** is an executable handler registered in `hooks.yaml`. A
CLI emits an event (e.g. `PreToolUse`); the per-CLI adapter
translates it into a nerv **envelope** (wire-shaped JSON); the
daemon dispatches the envelope to every matching handler; each
handler writes a JSON **decision** to stdout and signals intent via
**exit code**:

| Code | Action  | Behavior                            |
|------|---------|-------------------------------------|
| 0    | allow   | Event proceeds                      |
| 1    | warn    | Event proceeds; user sees message   |
| 2    | block   | Event rejected; user sees why       |
| 3    | rewrite | Event proceeds with modified payload|
| >=4  | error   | Fail-open: logged, event proceeds   |

The daemon merges multiple decisions for the same event using a
fixed priority: `block > rewrite (compose) > warn (accumulate) >
allow`. Failed handlers fall back to allow — fail-open is a hard
design principle, never a bug.

## The pipeline

```
CLI event → Adapter → nerv envelope → Dispatcher → Hook handler(s)
                                          ↓               ↓
                                     Merged decision ← stdout JSON
                                          ↓
                             Adapter → CLI-native response
```

The dispatcher is `nervd` — a daemon serving ConnectRPC's
`EventService` over a Unix domain socket. Adapters speak to it
through the matching SDK; one-shot CLI invocations
(`nerv test`, `nerv replay`) auto-spawn a transient dispatcher when
the socket isn't already up.

## Project state — `.nerv/` directory

| File | Purpose |
|---|---|
| `.nerv/hooks.yaml` | Hook registry. Each entry: `name`, `on:` (event), `match:` (regex), `run:` (handler path), optional `timeout`, `disabled`. |
| `.nerv/hooks.lock` | Resolved artifact + SHA-256 per hook. Written by `nerv add`. Don't hand-edit. |
| `.nerv/disabled.yaml` | Sidecar for event-level + binding-level disables. |
| `.nerv/events/*.jsonl` | Per-day event log. Replay/tail read from here. |

`hooks.yaml` is the project's source of truth. The daemon reloads
it on SIGHUP / `nerv daemon reload`; out-of-band edits aren't picked
up until reload.

## Daemon — `nervd`

**Status: stable.**

`nervd` is the always-on dispatcher. One binary, no config — it
reads `.nerv/hooks.yaml` from CWD and serves dispatches over a Unix
domain socket.

### Lifecycle

```sh
nerv daemon start     # detach + background, idempotent
nerv daemon status    # PID + uptime + socket path
nerv daemon stop      # SIGTERM then SIGKILL after 5s
nerv daemon reload    # SIGHUP — re-read hooks.yaml + disabled.yaml
```

### Socket path resolution

Resolved in this order:

1. `$NERV_SOCKET` (explicit override; absolute path).
2. `$XDG_RUNTIME_DIR/nerv.sock` (Linux/macOS with XDG set).
3. `$TMPDIR/nerv.sock` (macOS fallback).

The 104-byte `sun_path` limit on macOS is a real footgun — if your
home directory + project path push the socket path past 104 chars,
the daemon falls back automatically, but you can also set
`NERV_SOCKET` to a short path.

## Adapters

**Status: stable for `claude`, `codex`, `gemini`, `opencode`.
Pre-release for `openhands`, `aider`, `goose`, `vibe`.**

Each adapter knows how to translate one CLI's native hook surface
into nerv envelopes and decisions back into the CLI's native
response shape. Coverage varies — Claude Code emits all 26
PreToolUse-class events natively; Codex CLI only emits 3, so the
adapter synthesizes the rest via wrappers and watchers.

### Per-CLI install

```sh
nerv install add            # no CLI name = every detected CLI
nerv install add claude     # one CLI explicitly
nerv install add codex --dry-run
```

`nerv install add <cli>` writes the per-CLI config into wherever that
CLI expects it (e.g., `~/.claude/settings.json`,
`~/.codex/config.toml`, `~/.config/gemini/config.yaml`). Run
`nerv status` afterwards to confirm the wiring took.

### Capability maps

Each adapter ships a `capabilities.yaml` declaring which canonical
events it emits natively versus synthesizes. The
`spec/capabilities.schema.json` shape is the source of truth;
handlers branch on `envelope.synthesized` when they need to know.

Full per-adapter coverage matrix (✓ native / ~ synthesized / -
uncovered, all 8 adapters × 26 events):
[`docs/capability-matrix.md`](../../../docs/capability-matrix.md).

## Events — 36 canonical entries

**Status: stable.**

The event registry lives in `spec/events.yaml`. 36 entries grouped
by category. Every event declares `origin: native | derived |
extension` (default native). Highlights:

| Category | Events |
|---|---|
| `session` | SessionStart, SessionEnd, SessionStatus, SessionDiff |
| `prompt` | UserPromptSubmit |
| `tool` | PreToolUse, PostToolUse, PostToolUseFailure, BeforeToolSelection |
| `permission` | PermissionRequest, PermissionDenied |
| `agent` | SubagentStart, SubagentStop, TeammateIdle |
| `task` | TaskCreated, TaskCompleted |
| `context` | PreCompact, PostCompact, MemoryRead, MemoryWrite, InstructionsLoaded, CwdChanged, ConfigChange |
| `file` | FileChanged |
| `worktree` | WorktreeCreate, WorktreeRemove |
| `notification` | Notification, Stop, StopFailure |
| `bus` | inbox_message_added, inbox_message_updated, inbox_message_promoted, inbox_message_dropped |
| `mcp` | Elicitation, ElicitationResult |
| `lsp` | LspDiagnostics |

Full catalog with payload shapes + per-CLI native/synthesized
coverage: `references/event-catalog.md` and
`spec/events.yaml` upstream.

## Envelope (wire payload)

**Status: stable.**

Every event becomes an envelope:

```json
{
  "nerv_version": "0.1.0-alpha.0",
  "event": "PreToolUse",
  "event_id": "01HXYZ...",
  "timestamp": "2026-05-26T12:00:00Z",
  "cli": {"name": "claude", "version": "0.4.2"},
  "session": {"id": "abc-123", "project_key": "...", "cwd": "/path"},
  "payload": { "tool_name": "Bash", "tool_input": {...} },
  "synthesized": false
}
```

Max envelope size: 1 MB (soft, configurable via
`NERV_MAX_ENVELOPE_SIZE`); 10 MB hard ceiling. Envelopes over the
limit are rejected before reaching any handler.

For testing, synthetic envelopes are constructed by hand or via
`nerv test` (`--event`, `--tool`, `--command`). See
`references/envelope-shape.md` for the full field
reference.

## Decisions

**Status: stable.**

Handlers write one JSON object to stdout:

```json
{"action": "block", "message": "rm -rf / blocked", "metadata": {"rule": "danger"}}
```

| Field | Required | Notes |
|---|---|---|
| `action` | yes | One of: `allow`, `warn`, `block`, `rewrite`. |
| `message` | no | Human-readable; shown for warn/block. |
| `rewrite` | no | Replacement payload; action must be `rewrite`. |
| `metadata` | no | Arbitrary KV persisted in the event log. |

Exit code wins on mismatch with `action` — dispatcher logs the
mismatch at warn level. Empty stdout is treated as `{"action":
"allow"}`.

Fail-open is enforced at every layer: if the handler crashes,
times out (default 30s, max 5min), exits ≥ 4, or writes garbage to
stdout, the dispatcher logs the error and treats that hook as
`allow`. Other hooks for the same event still run normally.

## Lifecycle: author → install → test → enable/disable → tail/replay

### 1. Author a hook

Two paths.

**Hand-written.** Drop an executable into `./hooks/`, add a
`hooks.yaml` entry pointing at it.

```yaml
# .nerv/hooks.yaml
version: "1.0-draft"
hooks:
  - name: block-rm-rf
    on: PreToolUse
    match: "Bash|shell|local_shell"
    run: ./hooks/block-rm-rf.sh
```

```sh
#!/bin/bash
# hooks/block-rm-rf.sh — exits 2 + writes block decision on match
CMD=$(jq -r '.payload.tool_input.command // empty')
if echo "$CMD" | grep -qE 'rm\s+-rf\s+/'; then
  echo '{"action":"block","message":"rm -rf / blocked by policy"}'
  exit 2
fi
exit 0
```

**LLM-assisted via `/nerv-create` or `nerv create`.** Both take a
natural-language description, generate a handler script + a
`hooks.yaml` entry via the configured LLM provider
(Anthropic / OpenAI / local), and drop them into the project.

### 2. Install + adapters

```sh
nerv install add        # wire up every detected CLI
nerv daemon start
nerv add block-rm-rf    # marketplace lookup, lockfile pin (/nerv-add: picker)
```

`nerv add` resolves the named hook (from the curated marketplace or
a local path), copies the handler artifact, registers it in
`hooks.yaml`, and pins the resolved SHA-256 in `hooks.lock`.

### 3. Test before letting real traffic in

```sh
nerv test block-rm-rf --event PreToolUse --tool Bash \
  --command "rm -rf /"
```

The synthetic envelope flows through the same dispatch path as a
live event; the handler runs, the decision merges, but nothing
leaks back to a CLI. Useful for verifying a hook fires on the
right inputs and skips on the wrong ones.

### 4. Enable / disable

Three units of disable (`nerv disable`, or `/nerv-disable`):

| Unit | Persists via | Effect |
|---|---|---|
| **Hook** | `disabled: true` on the `hooks.yaml` entry | Daemon skips this hook for any event. |
| **Event** | Entry in `.nerv/disabled.yaml` under `events:` | All hooks for that event are skipped. |
| **Binding** | Entry in `.nerv/disabled.yaml` under `bindings:` (hook + event pair) | Skips one specific (hook, event) combination. |

On match, the daemon emits `{decision: skipped, reason: disabled}`
into the event log and does NOT invoke the handler. Disables take
effect after `nerv daemon reload`. The symmetric `nerv enable`
removes the marker.

### 5. Tail (live) and replay (forensics)

```sh
nerv tail                                # follow the event log
nerv tail --event PreToolUse --format json  # filter + structured
nerv replay <event-id>                   # re-dispatch a recorded envelope
nerv replay --last 5                     # re-dispatch the last 5 events
```

`tail` is for watching what's happening right now. `replay` is for
asking "what would today's hook config have decided about
yesterday's event?" — useful for regression checks and incident
forensics.

## Doctor + diagnostics

**Status: stable.**

```sh
nerv doctor             # probe everything, report PASS/WARN/FAIL
nerv doctor -V          # extra detail
```

Probes (read-only by default):

- Daemon process + socket reachability
- `hooks.yaml` schema validity
- `hooks.lock` consistency with on-disk artifacts (SHA-256 check)
- Per-CLI adapter install state
- `capabilities.yaml` compatibility against detected CLI versions
- Event log writability + rotation health

`nerv status` is a quicker compact summary aimed at agents and CI.

## Slash commands shipped by this plugin

All seven live under `.agents/commands/`.

| Command | One-line purpose |
|---|---|
| `/nerv-add` | Add a hook to the project (marketplace picker on bare invocation). |
| `/nerv-create` | LLM-generated handler script + `hooks.yaml` entry from a description. |
| `/nerv-disable` | Multi-select disable across hooks, events, and bindings (with `/nerv-enable` symmetric). |
| `/nerv-doctor` | Run the health check; interactive `--fix` on WARN/FAIL. |
| `/nerv-install` | Install or refresh a per-CLI adapter (multi-select picker for `--all`). |
| `/nerv-replay` | Replay a recorded event by ID through the current hook config. |
| `/nerv-test` | Run a hook against a synthetic envelope (interactive payload builder). |

Each slash command in this plugin shells out to the matching `nerv`
CLI verb and adds Claude-side affordances (interactive pickers,
prompt-driven envelope construction, fix prompts).

## Before you begin

- `nerv` binary on `$PATH`
  (`go install hop.top/nerv/cmd/nerv@latest` plus `cmd/nervd`,
  available after the first tagged release).
- A running daemon — `nerv daemon start` once, then `nerv doctor`
  to confirm.
- At least one adapter installed for the CLI you're using
  (`nerv install add` auto-detects).
- Project initialized with a `.nerv/hooks.yaml` (created on first
  `nerv add` if missing).

## Common pitfalls

| Symptom | Likely cause | Fix |
|---|---|---|
| `daemon: connection refused` | `nervd` not running | `nerv daemon start` then retry |
| `bind: name too long` on macOS | UDS `sun_path` > 104 bytes | Set `NERV_SOCKET` to a short path, e.g. `/tmp/n.sock` |
| Hook landed but never fires | Daemon hasn't reloaded after `hooks.yaml` edit | `nerv daemon reload` |
| `disabled: true` ignored | Same — daemon needs reload | `nerv daemon reload` |
| `hooks.lock` SHA mismatch | Hand-edited handler script | `nerv add --refresh <hook>` to re-pin; never hand-edit `hooks.lock` |
| Replay finds no event | Wrong event-id or log rotated | `nerv tail --last 50` to surface IDs; check `.nerv/events/` |
| Codex hook never runs | Adapter coverage gap (Codex emits 3/26 natively) | Check `nerv doctor -V` and `docs/capability-matrix.md`; some events need adapter synthesis to be enabled |
| Doctor reports adapter as WARN | `capabilities.yaml` annotated against pre-v0.1 4-tier side-effect taxonomy (6-tier draft pending) | Safe to ignore for now; WARN clears when the taxonomy upgrade lands |

## Related references

- `references/hooks-yaml-schema.md` — field-by-field reference for
  `.nerv/hooks.yaml`.
- `references/event-catalog.md` — full 36-event catalog with
  payload shapes and per-CLI native/synthesized coverage.
- `references/envelope-shape.md` — wire format details, size
  limits, redaction semantics.
- Main repo: [`../../../README.md`](../../../README.md) — nerv
  overview, install matrix, SDK list.
- [`../../../spec/events.yaml`](../../../spec/events.yaml) —
  canonical event registry.
- [`../../../spec/envelope.schema.json`](../../../spec/envelope.schema.json) —
  envelope JSON Schema.
- [`../../../spec/hooks.schema.json`](../../../spec/hooks.schema.json) —
  `hooks.yaml` JSON Schema.
- [`../../../spec/handler-contract.md`](../../../spec/handler-contract.md) —
  handler protocol (exit codes, merge semantics, env vars).
- [`../../../spec/capabilities.schema.json`](../../../spec/capabilities.schema.json) —
  per-adapter capability map shape.
- [`../../../spec/redaction.md`](../../../spec/redaction.md) —
  secret redaction defaults + opt-in to full payloads.
- [`../../../spec/failure-modes.md`](../../../spec/failure-modes.md) —
  12-entry failure matrix.
- [`../../../spec/socket-protocol.md`](../../../spec/socket-protocol.md) —
  daemon / UDS protocol details.
- [`../../../docs/architecture.md`](../../../docs/architecture.md) —
  high-level architecture.
- [`../../../docs/RELEASING.md`](../../../docs/RELEASING.md) —
  release-please + publish-on-tag fanout for the polyglot SDKs.
