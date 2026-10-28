# ADR: crtx v0.1 producer — event mapping, topics, state layout

Last updated: 2026-08-07
Author: $USER
Status: Proposed

## Context

The crtx v0.1 spec (<https://spec.hop.top/crtx/v0.1/envelope.schema.json>)
defines the interchange shape for AI agent conversations: an Envelope of
ordered Turns made of ContentParts, plus a bus event taxonomy
(`crtx.<category>.<object>.<action>`) whose central invariant is that
**every event payload IS a valid ContentPart or Envelope metadata
fragment** — a recorder appends to an Envelope without translation.

nerv today produces zero crtx output. The hook envelope
(`internal/envelope/envelope.go`) is a hook-event record — event name,
event id, CLI, session, opaque payload — not a conversation container.
The daemon instantiates kit/bus but nothing publishes. Downstream
indexers already treat this tool as a well-known crtx producer and probe
`$XDG_STATE_HOME/nerv/sessions/*.jsonl` for envelope JSONL (one file per
session, the same convention stem's JSONLStore uses).

This ADR decides (1) how the ~36 nerv hook events map onto crtx topics,
(2) payload construction rules, (3) the on-disk state layout that keeps
internal logs and crtx sessions in distinct subtrees, and (4) the Go
type surface used to produce envelopes.

## Decision drivers

- crtx §2 (events.md): the `crtx.*` namespace is reserved; producers
  MUST NOT invent topics under it. Extensions MUST use `x.<vendor>.*`.
- kit/bus validates published topics as exactly four lowercase
  dot-separated segments (`[Source].[Category].[Object].[Action]`);
  default enforcement is warn-mode. Every topic we emit must satisfy
  both grammars.
- Hook events carry no assistant text and no tool-call correlation id;
  the mapping must be honest about what the hook layer can see.
- Fail-open invariant: bus/export failure must never affect dispatch
  decisions.
- Consumers scan `$XDG_STATE_HOME/<producer>/sessions/`; internal
  eventlog files must never share that namespace.
- crtx is Draft/alpha; producer pins `crtx_version: "0.1"` and treats
  schema validation as the only conformance enforcement available.

## Decisions

### 1. Topic grammar and namespaces

- Canonical conversation events publish on the exact crtx v0.1 topic
  names from events.md §3. No invented `crtx.*` topics.
- Everything nerv-specific publishes under the vendor prefix `x.nerv.*`,
  shaped as four segments `x.nerv.<object>.<action>` so one topic
  satisfies the crtx extension rule (§5) and kit/bus grammar
  simultaneously.
- The inbox lifecycle topics planned by the guardrails escalation path
  (previously sketched as bare `inbox_message_added`) become:
  `x.nerv.inbox_message.added`, `x.nerv.inbox_message.updated`,
  `x.nerv.inbox_message.promoted`, `x.nerv.inbox_message.dropped`.
  The nerv *event names* in `spec/events.yaml` are unchanged — only the
  bus topic form is fixed.
- Bus `Event.Source` is `nerv` for every publish from this codebase.

### 2. Event mapping table

| nerv event | crtx topic | Notes |
|-----------|------------|-------|
| `SessionStart` | `crtx.session.envelope.created` | envelope id = session id; source from CLI info (§3) |
| `SessionEnd` | `x.nerv.session.ended` | v0.1 has no lifecycle terminator (events.md §9); vendor topic |
| `UserPromptSubmit` | `crtx.turn.user.received` | content = single `text` part from `prompt` |
| `PreToolUse` | `crtx.turn.tool.called` | call id per §3 correlation rule |
| `PostToolUse` | `crtx.turn.tool.completed` | output = `tool_response` |
| `PostToolUseFailure` | `crtx.turn.tool.completed` | `is_error: true`, output = error string |
| `Stop` | `crtx.turn.assistant.emitted` | content = extension part (§3); hooks never see assistant text |
| `StopFailure` | `x.nerv.turn.failed` | crtx error topics deferred to v0.2 |
| `SubagentStart` | `crtx.session.envelope.nested` | child id = `agent_id`; synthetic call id (§3) |
| `SubagentStop` | `crtx.session.envelope.returned` | same synthetic call id; `is_error` from exit code |
| `Notification` | `x.nerv.notification.emitted` | no crtx counterpart |
| `PermissionRequest` | `x.nerv.permission.requested` | no crtx counterpart |
| `PermissionDenied` | `x.nerv.permission.denied` | no crtx counterpart |
| `TaskCreated` | `x.nerv.task.created` | no crtx counterpart |
| `TaskCompleted` | `x.nerv.task.completed` | no crtx counterpart |
| `inbox_message_*` | `x.nerv.inbox_message.*` | nerv-origin bus events (§1) |
| `TeammateIdle` | drop | agent-pool telemetry, not conversation data |
| `InstructionsLoaded` | drop | workspace telemetry |
| `ConfigChange` | drop | workspace telemetry |
| `CwdChanged` | drop | workspace telemetry |
| `FileChanged` | drop | workspace telemetry |
| `WorktreeCreate` / `WorktreeRemove` | drop | workspace telemetry |
| `PreCompact` / `PostCompact` | drop | host-runtime mechanics |
| `Elicitation` / `ElicitationResult` | drop | MCP UI mechanics; revisit if crtx grows an elicitation shape |
| `BeforeToolSelection` | drop | pre-decision; no Turn exists yet |
| `LspDiagnostics` / `SessionDiff` / `SessionStatus` | drop | single-CLI extension telemetry |
| `MemoryRead` / `MemoryWrite` | drop | derived from `PostToolUse`, which already maps; exporting both would double-emit the same tool call |

"Drop" means: not exported to the crtx surface. Dropped events still
flow through hook dispatch and the internal eventlog unchanged.

### 3. Payload construction rules

- `envelope_id` = `envelope.session.id`. One CLI session = one crtx
  Envelope.
- `turn_id` = `envelope.event_id` (adapter-unique per event).
- **Tool-call correlation.** The hook wire format has no call id and no
  Pre→Post correlation key. `call_id` is derived deterministically:
  `c<hex(sha256(session_id ␟ tool_name ␟ canonical(tool_input)))[:24]>`
  where `canonical` is key-sorted JSON. `PreToolUse` and
  `PostToolUse`/`PostToolUseFailure` for the same invocation produce the
  same digest, so `tool.called`/`tool.completed` pair without state and
  the scheme works in both stateless and daemon modes. Limitation:
  byte-identical repeated calls within one session collide; acceptable
  for an observational feed (events are best-effort per events.md §4 —
  authoritative state is the Envelope) and documented here rather than
  papered over with in-memory state a stateless invocation cannot hold.
- **Subagent correlation.** `SubagentStart`/`SubagentStop` carry no
  spawning `tool_call`. The nest/return pair uses a synthetic
  `call_id` = `n<hex(sha256(parent_id ␟ child_id))[:24]>`, with
  `parent_id` = `parent_session_id` (fallback: current session id) and
  `child_id` = `agent_id`.
- **Assistant content.** `Stop` fires when the model finishes a turn;
  the hook layer receives `stop_reason`/`turn_index`, never the
  response text. The `assistant.emitted` content array is a single
  extension part `{"type": "x-io.hop-top.nerv.stop", "stop_reason": …,
  "turn_index": …}` — schema-valid (envelope.md §6.6) and honest about
  provenance. Consumers wanting full assistant text read the producing
  CLI's own transcript; this feed contributes timing, tool activity,
  and prompts.
- **Source.** `crtx.session.envelope.created.source` is built from the
  hook CLI info: kind = CLI name normalized to the spec-recommended
  vocabulary (`claude` → `claude-code`, `gemini` → `gemini-cli`, others
  pass through verbatim — kind is an open string), version = CLI
  version (`unknown` when the adapter could not detect one), instance
  omitted.
- Timestamps: RFC 3339 from the hook envelope timestamp.
- `crtx_version` is pinned to `"0.1"` everywhere.

### 4. Vendor payload shape (`x.nerv.*`)

Extension payloads are caller-defined (events.md §5). All `x.nerv.*`
topics carry one uniform shape:

```json
{
  "envelope_id": "<session id>",
  "event": "<nerv event name>",
  "event_id": "<hook event id>",
  "occurred_at": "<RFC 3339>",
  "payload": { …verbatim nerv event payload… }
}
```

Uniformity keeps subscribers generic; the embedded `payload` preserves
the catalog shape from `spec/events.yaml` without re-modelling it.

### 5. Recorder and session store

A recorder subscribes to `crtx.turn.#`, `crtx.session.#`, and
`x.nerv.session.#` on the daemon bus and appends to envelope JSONL —
possible without translation because canonical payloads are ContentParts
/ metadata fragments by spec invariant. Store convention matches stem's
JSONLStore so indexers consume with zero changes: one file per session
at `<sessions-dir>/<envelope-id>.jsonl`, exactly one line holding the
complete Envelope, atomically rewritten (temp file + fsync + rename) on
every append.

Folding rules:

- `envelope.created` → create envelope file.
- `user.received` → append `user` Turn.
- `assistant.emitted` / `thinking.emitted` → append `assistant` Turn.
- `tool.called` → append `assistant` Turn holding the `tool_call` part
  (envelope.md §6.2: calls are issued by the assistant).
- `tool.completed` → append `tool` Turn holding the `tool_result` part.
- `system.injected` → append `system` Turn.
- `x.nerv.session.ended` → record exit metadata on the Envelope
  (`metadata["io.hop-top.nerv.session_end"]`) and bump `updated_at`.
- `envelope.nested` / `envelope.returned` are published but not folded:
  the child session materializes its own Envelope from its own hook
  stream, and the parent transcript has no real `tool_call` to close.
  Cross-linking via `dispatched_from` needs correlation the hook layer
  does not provide today; deferred.

A turn event arriving for an unknown envelope auto-creates a skeleton
envelope (source kind `custom`, version `unknown`, metadata flag
`io.hop-top.nerv.synthesized: true`) so mid-session daemon starts and
stateless invocations never drop turns.

Recorder failures are logged and never propagate into dispatch
(fail-open).

### 6. Type surface: local declaration, no runtime SDK import

The producer declares the crtx v0.1 type surface locally in
`internal/crtx` instead of importing the stem Go SDK:

- Precedent: the vein indexer deliberately re-declares the same surface
  for the reading side; both sides treat the JSON Schema as the
  contract, not any Go module.
- Importing the stem SDK would couple this repo's hot path to an
  unrelated agent runtime's release cadence for ~150 lines of structs,
  and would still not be the long-term answer — the agreed eventual
  home is a spec-crtx SDK package, which does not exist yet.
- Schema validation in tests (vendored `envelope.schema.json` /
  `events.schema.json`) pins the local types to the spec; drift fails
  CI, same guarantee an SDK import would give.

When a spec-crtx SDK ships, `internal/crtx` types migrate to it; the
store/recorder stay.

### 7. State layout — segregate eventlog from the session scan root

```
$XDG_STATE_HOME/nerv/
  events/                    internal surface (never scanned by indexers)
    events-YYYY-MM-DD.jsonl  hook eventlog (nerv tail / replay / doctor)
    bus.jsonl                default jsonl bus sink file
    bus.sqlite               default sqlite bus adapter file
  sessions/                  interchange surface (crtx envelopes only)
    <envelope-id>.jsonl      one crtx v0.1 Envelope per session
```

- crtx consumers scan `<state>/nerv/sessions/` — nothing else of ours.
  Internal Entry JSONL lives under `events/`; the two surfaces can
  never collide regardless of file naming.
- The eventlog default directory moves from the flat
  `$XDG_STATE_HOME/nerv/` root into the segregated
  `$XDG_STATE_HOME/nerv/events/` subtree so the state root can host
  the sessions/ interchange surface without namespace collisions.
  Migration: move `events-*.jsonl` from the state root into
  `$XDG_STATE_HOME/nerv/events/` (see
  `docs/migrations/state-dir-layout.md`).
- Bus sink defaults follow the same split: config-selected `jsonl` /
  `sqlite` artifacts default into `events/`.

### 8. Spec stability and pinning

crtx v0.1 is Draft/alpha and self-declares unstable; its conformance
suite is planned but not published. Producer policy:

- Pin `crtx_version: "0.1"`; never emit a version the schemas cannot
  back.
- Vendor `envelope.schema.json` + `events.schema.json` (and the spec's
  published examples) as testdata; unit tests validate every emitted
  payload and every persisted envelope against them. Vendored copies
  record their upstream origin and must be refreshed when the spec
  tags a new v0.1 revision.
- On a breaking v0.2, the producer emits the new version behind a new
  pin in one change; consumers reject on `crtx_version` mismatch per
  envelope.md §8, so mixed deployments degrade to skipped files, not
  corruption.

## Consequences

### Positive

- Sessions become consumable by any crtx v0.1 reader with zero
  consumer-side changes (stem JSONLStore file convention, scanned
  directory layout, schema-valid envelopes).
- Bus surface is spec-clean: canonical topics only under `crtx.*`,
  everything else explicitly vendor-prefixed; kit/bus warn-mode
  validation stays silent.
- Dropped-event policy keeps the interchange feed conversation-shaped
  instead of mirroring the whole hook firehose.

### Negative

- Deterministic call ids collide for byte-identical repeated tool calls
  in one session; pairing is best-effort by design.
- `assistant.emitted` carries no assistant text — a structural limit of
  hook-layer observation, now an explicit part of the contract.
- Two envelope vocabularies coexist in the codebase (hook envelope vs
  crtx Envelope); naming requires care (`internal/envelope` vs
  `internal/crtx`).

### Risks

- crtx is alpha: a v0.1 schema revision upstream can invalidate the
  vendored copies. Mitigation: §8 refresh policy; validation lives in
  tests so drift is caught at CI, not in production.
- Session-id reuse across CLIs would merge unrelated conversations into
  one envelope file. Session ids observed from adapters are UUIDs;
  accepted.

## References

- crtx v0.1 spec — envelope: <https://spec.hop.top/crtx/v0.1/envelope.schema.json>;
  events: <https://spec.hop.top/crtx/v0.1/events.schema.json>
- stem (JSONLStore convention): <https://github.com/hop-top/poly-stem>
- vein (consumer-side scan + type re-declaration): <https://github.com/hop-top/vein>
- kit/bus adoption ADR — [`kit-bus-adoption.md`](kit-bus-adoption.md)
- Event catalog — [`spec/events.yaml`](../../spec/events.yaml)
- Hook envelope — [`internal/envelope/envelope.go`](../../internal/envelope/envelope.go)
- Eventlog — [`internal/eventlog/jsonl.go`](../../internal/eventlog/jsonl.go)
- Failure modes — [`spec/failure-modes.md`](../../spec/failure-modes.md)
