# ADR: crtx v0.1 consumer — ingestion, routing, inbox delivery

Last updated: 2026-08-08
Author: $USER
Status: Proposed

## Context

The producer ADR ([`crtx-producer.md`](crtx-producer.md)) made this
tool emit crtx v0.1: hook envelopes translate onto canonical
`crtx.*` / vendor `x.nerv.*` bus topics, and a recorder folds the
emissions into envelope JSONL under `$XDG_STATE_HOME/nerv/sessions/`.

That is exactly half of the "signal router" this tool claims to be.
Today there is zero consume path: nothing subscribes to external
producers, nothing reads a foreign producer's session files, and an
inbound turn from another agent has nowhere to land.

An honest inventory of the "inbox" this codebase keeps referring to:

- `internal/crtx` declares the four `x.nerv.inbox_message.<action>`
  bus topics; nothing publishes them.
- `spec/events.yaml` catalogs the `inbox_message_*` payload shapes
  (msg_id, inbox_id, kind, payload_ref, …); no code constructs them.
- The guardrails escalation matcher
  (`internal/hookpkg/builtins/guardrails/escalate.go`) documents an
  inbox-based approval flow and stubs it to Block.
- The builtin runtime config carries an `InboxDir` field no builtin
  writes to; the open-questions builtin defaults an `inbox_path` it
  never uses.
- Stories US-0003 / US-0005 describe inbox writes and a triage
  workflow; neither is implemented.

The inbox is vocabulary, not infrastructure. This ADR therefore
decides (1) which inputs the consumer ingests, (2) what "routing"
concretely means, (3) the minimal honest inbox delivery mechanism the
routed turns land in, (4) the trust/validation boundary, and (5)
idempotency and replay rules.

## Decision drivers

- kit/bus is the only transport. The network adapter already injects
  remote peers' events into the local bus as ordinary publishes with
  the originator's `Event.Source` preserved; consuming means
  subscribing, never re-implementing transport.
- The established crtx reader discipline (the vein indexer, mirrored
  in `internal/crtx/interop_test.go`): scan roots one directory level
  deep for `*.jsonl`, read the last non-empty line, gate on
  `crtx_version`, skip unparseable files.
- envelope.md §8: consumers MUST reject envelopes whose
  `crtx_version` major is unsupported. envelope.md §4: unknown roles
  MUST be rejected, not coerced. events.md §5: extension payloads are
  caller-defined; §7: subscribers tolerate missing optional fields.
- events.md §4: events are best-effort and may be observed more than
  once (and via multiple topics); consumers own their dedup.
- Fail-open invariant (`spec/failure-modes.md`): ingestion is a side
  channel; a malformed or hostile inbound event must never crash the
  daemon or affect dispatch decisions.
- The producer publishes on the same daemon bus the consumer
  subscribes to; without an origin filter the router would ingest its
  own emissions in a loop.
- crtx v0.1 is Draft/alpha with no published conformance suite;
  vendored schemas are the only enforceable contract.

## Decisions

### 1. Two ingest paths, both through existing machinery

**Bus subscriptions.** In daemon mode the consumer subscribes on the
daemon bus (whatever adapter `bus.adapter` selected — for cross-agent
traffic that is `network`) to:

- `crtx.turn.#` — canonical turn events, the routable payload.
- `crtx.session.#` — session lifecycle, consumed for accounting only
  in this revision (correlation uses envelope-level linkage, §6).
- `x.*.#` — extension events, accounted and never routed (§5).

Remote producers reach this surface for free: the kit/bus network
adapter re-publishes inbound peer events on the local bus, so one
subscription covers in-process and networked origins alike.

**Foreign session JSONL scan.** A polling scanner walks configured
scan roots with the reader discipline above (one level deep,
last-line read, version gate). Default root: the sibling stem
producer's convention directory `$XDG_STATE_HOME/stem/sessions`,
scanned only when it exists. `ingest.scan_roots` in the config
replaces the default; `ingest.scan_interval_seconds` tunes the poll
(default 5). Scanning is change-driven: a file whose (size, mtime)
pair is unchanged since the last pass is not re-read; dedup (§6)
makes the occasional redundant read harmless.

Config shape (all keys optional):

```yaml
ingest:
  enabled: true               # default true; false disables both paths
  scan_roots:
    - /abs/path/to/sessions   # replaces the default stem root
  scan_interval_seconds: 5
```

Stateless invocations never ingest: consumption is a daemon concern.

### 2. What "routing" means

Routing = delivering a foreign conversational turn into the local
inbox store (§3), with source attribution and lineage correlation,
exactly once per logical turn (§6). It is transport fan-in to a
readable surface — not semantic filtering, not forwarding, not
transformation (all explicitly out of scope here).

Routing policy:

| Input | Routed? | Rationale |
|-------|---------|-----------|
| `crtx.turn.user.received` | yes | inbound message |
| `crtx.turn.assistant.emitted` | yes | inbound message |
| `crtx.turn.system.injected` | only with `in_reply_to_call_id` | mid-flight interjection (envelope.md §3.2); plain system prompts are transcript context, not messages |
| `crtx.turn.thinking.emitted` | no (accounted) | droppable per events.md §3.5 |
| `crtx.turn.tool.called` / `.completed` | no (accounted) | call mechanics, not messages |
| `crtx.session.*` | no (accounted) | lifecycle, not messages |
| `x.<vendor>.*` | no (accounted) | opaque payloads (§5) |
| scanned envelope turns, role `user` / `assistant` / `developer` | yes | same message test |
| scanned envelope turns, role `system` | only with `in_reply_to_call_id` | as above |
| scanned envelope turns, role `tool` | no (accounted) | as above |

The test is "is this a message another party produced", not "is this
part of a transcript". The inbox is a notification surface; the
authoritative transcript stays in the producer's envelope (events.md
§8). Volume control beyond this role gate is the semantic-filtering
follow-up, deliberately not designed here.

### 3. The inbox: a file-backed store, lifecycle events, a read command

Because the inbox is vocabulary today (see Context), this ADR defines
the minimal mechanism that makes delivery real without inventing UX:

**Store.** One JSON document per message at

```
$XDG_STATE_HOME/nerv/inbox/<inbox-id>/<msg-id>.json
```

written atomically (temp + fsync + rename, the session-store
discipline). Foreign turns land in inbox id `foreign-turns`. The
directory layout intentionally matches the container/item split the
event catalog already encodes (`inbox_id` + `msg_id`), so the
guardrails escalation flow and the open-questions builtin can adopt
sibling containers later without migration.

**Entry shape** (`schema_version: "1.0"`):

```json
{
  "schema_version": "1.0",
  "msg_id": "m<hex24>",
  "inbox_id": "foreign-turns",
  "kind": "foreign-turn",
  "received_at": "<RFC 3339>",
  "origin": {
    "transport": "bus | scan",
    "topic": "<bus topic, bus path>",
    "bus_source": "<Event.Source, bus path>",
    "path": "<session file, scan path>",
    "source": { "kind": "…", "version": "…" }
  },
  "envelope_id": "<foreign envelope id>",
  "turn_id": "<foreign turn id>",
  "role": "user | assistant | developer | system",
  "agent_id": "<optional>",
  "in_reply_to_call_id": "<optional>",
  "correlation": {
    "local_session_id": "<optional>",
    "relation": "parent | dispatched_from"
  },
  "content": [ …ContentParts verbatim… ],
  "preview": "<first text part, truncated>"
}
```

`content` preserves the foreign ContentParts byte-for-byte (unknown
part types stay intact per envelope.md §6.6). `origin.source` is the
foreign envelope's Source on the scan path; the bus path has no
envelope in hand and records topic + `Event.Source` instead — an
honest reflection of what each path can actually see.

**Lifecycle events.** Every delivery publishes
`x.nerv.inbox_message.added` on the daemon bus with the payload shape
`spec/events.yaml` already catalogs: `schema_version`, `msg_id`,
`inbox_id`, `source_session_id` (the foreign envelope id), `kind`,
`ts`, `payload_ref` (the entry file path), plus `agent_id` when an
active profile is resolvable. This is the first real publisher of the
inbox vocabulary; `updated` / `promoted` / `dropped` remain with the
triage workflow (US-0005) that owns those transitions.

**Read surface.** A read-only `inbox` command group on the CLI:
`nerv inbox list` (newest first, preview column) and
`nerv inbox show <msg-id>` (full entry). Both honor the standard
output formats. No triage verbs here — promote/drop belong to the
US-0005 workflow and would be dishonest to ship as stubs. The files
themselves are plain JSON precisely so `$EDITOR`, `jq`, or any agent
can read them without the CLI.

### 4. Self-origin filtering

- **Bus path:** every event published from this codebase carries
  `Event.Source == "nerv"` (producer ADR §1). The consumer drops
  `Source == "nerv"` events before validation, with accounting. This
  breaks the local loop (dispatch publish → consumer) definitively.
- **Scan path:** roots that resolve inside `$XDG_STATE_HOME/nerv/`
  are rejected at scanner construction with a logged warning — the
  producer's own `sessions/` subtree can never feed the scanner,
  regardless of configuration mistakes.
- **Known limitation:** the source filter also drops events from a
  *remote* nerv peer, because kit/bus does not expose the network
  origin identity to handlers (the wire `origin` field is consumed by
  the adapter's own loop prevention and not surfaced on `Event`).
  Remote nerv-to-nerv routing is deferred until the bus event schema
  carries an origin/instance identity; a remote nerv's sessions
  remain reachable via the scan path meanwhile.

### 5. Trust and validation boundary

Everything inbound is untrusted. The boundary, in evaluation order:

1. **Self-origin gate** (§4) — dropped, accounted as `self`.
2. **Topic class.** Canonical `crtx.turn.*` proceeds; everything else
   is accounted (`session`, `extension`) and never routed. Unknown
   vendors under `x.*` are not an error — they are opaque by spec
   (events.md §5) and simply not inbox material.
3. **Schema validation.** Canonical payloads validate against the
   corresponding `$defs` entry of the vendored `events.schema.json`
   before acceptance; scanned files validate the whole envelope
   against the vendored `envelope.schema.json`. The vendored copies
   move from test-only fixtures to compiled-in resources so the
   runtime enforces exactly what the tests pin. Validation failure →
   drop with account (`invalid`), log line, never a crash, never a
   partial delivery.
4. **Version gate.** Scanned envelopes with `crtx_version` other
   than the pinned `0.1` are skipped (`version` account) per
   envelope.md §8. Bus payloads carry no version field; the topic
   grammar + schema validation are the enforceable equivalent.
5. **Role gate.** Turns whose role is outside the v0.1 enum reject
   the envelope (envelope.md §4: reject, don't coerce) — accounted
   as `invalid`.
6. **Routing policy** (§2) — non-message turns accounted as
   `skipped`.

Accounting is a set of named counters (accepted, self, invalid,
version, skipped, extension, session) logged on change and exposed to
tests; a later change can surface them through `nerv doctor` without
reshaping the boundary.

### 6. Idempotency, dedup, replay

- **Message identity.** `msg_id` is derived deterministically:
  `m<hex(sha256(envelope_id ␟ turn_id))[:24]>` (same digest
  discipline as the producer's call ids). The turn id is the event
  identity on the bus (`turn_id`) and in scanned envelopes
  (`Turn.id`), so the *same logical turn observed via both paths* —
  live on the bus, then again in the producer's session file —
  yields the same `msg_id` and delivers exactly once.
- **The store is the ledger.** Delivery is `O_EXCL`-style: an entry
  whose `msg_id` file already exists is a silent no-op (accounted as
  duplicate, no `inbox_message.added` re-publish). No side index to
  corrupt or rebuild; restart-safe by construction.
- **Replay safety.** Producer-side replays (`nerv replay`, recorder
  re-folds, scanner re-reads after restart) re-observe events; the
  dedup above makes redelivery a no-op. This satisfies the events.md
  §4 requirement that consumers tolerate repeated observation.
- **Collision honesty.** Distinct foreign producers using colliding
  `(envelope_id, turn_id)` pairs would dedup against each other.
  Envelope ids are spec-recommended ULIDs/UUIDs; accepted, same
  trade-off class as the producer's deterministic call ids.

### 7. Recorder scope narrows to own emissions (surgical producer touch)

The existing session recorder subscribes to `crtx.turn.#` /
`crtx.session.#` without an origin filter. The moment a network
adapter feeds foreign events into the daemon bus, the recorder would
fold *foreign* conversations into `$XDG_STATE_HOME/nerv/sessions/` as
synthesized skeleton envelopes — duplicating the foreign producer's
own transcript under this producer's identity, and polluting the
interchange surface indexers scan (the same conversation would be
indexed twice with divergent content).

Decision: the recorder folds only events with
`Event.Source == "nerv"`. The consumer (this ADR) owns foreign
events; the recorder owns the producer's own. This is the one
producer-side change this track makes, and it is a scope *narrowing*
that preserves every existing behavior for locally produced events.

### 8. State layout addition

```
$XDG_STATE_HOME/nerv/
  events/                    internal surface (unchanged)
  sessions/                  interchange surface (unchanged, own sessions only)
  inbox/                     delivery surface (new)
    foreign-turns/           container for routed foreign turns
      <msg-id>.json          one delivered message per file
```

The inbox subtree is neither an interchange surface (never scanned by
crtx indexers — it holds inbox entries, not envelopes) nor internal
telemetry; it is the local delivery surface the CLI and local agents
read. Consumers of *this* producer's crtx output continue to scan
`sessions/` only.

### 9. Spec stability

Identical policy to producer ADR §8: pin `crtx_version "0.1"`,
vendored schemas are the contract, refresh on upstream v0.1
revisions, reject-on-mismatch keeps mixed deployments degraded rather
than corrupted. The consumer adds one nuance: rejection is now a
*runtime* behavior (skip + account), not merely a CI assertion, which
is precisely what envelope.md §8 asks of consumers. Coverage limits
while the upstream conformance suite remains unpublished are
documented in the interop test suite.

## Consequences

### Positive

- The "signal router" billing becomes true end to end: a foreign
  agent's turn arrives over the bus or lands in a scanned session
  file and becomes a readable inbox entry with attribution, lineage,
  and a lifecycle event other tools can subscribe to.
- The inbox vocabulary (topics, catalog shapes, `InboxDir`) gains its
  first real implementation, giving the guardrails escalation and
  open-questions flows a concrete store to adopt.
- One dedup identity across both ingest paths means bus-vs-scan
  overlap is a feature (lower latency live, completeness from files)
  instead of a duplication bug.
- The trust boundary is enforced with the same vendored schemas the
  producer tests already pin — one contract, both directions.

### Negative

- Every routed foreign turn is one inbox file; a chatty foreign
  session produces many entries. Acceptable for v1 (the role gate
  drops mechanics), and the declared follow-up for volume shaping is
  semantic filtering, not this layer.
- Remote nerv peers are invisible to the bus path (§4 limitation)
  until kit/bus exposes origin identity on events.
- The bus path cannot attribute a producer kind/version (no envelope
  in hand); `origin.bus_source` + topic is the best honest
  attribution available live.

### Risks

- A scan root pointed at a huge directory makes the poll expensive.
  Mitigated by the change-driven (size, mtime) skip and the
  one-level walk; unbounded roots remain an operator choice.
- Foreign envelope files are rewritten whole by their producers; a
  read racing a non-atomic foreign writer can observe a torn file.
  The validation boundary treats it as `invalid` this pass and picks
  the file up on the next change tick — self-healing, but a turn's
  delivery can lag one interval.
- crtx alpha drift: an upstream v0.1 schema revision changes what
  validates. Same mitigation as the producer: vendored copies +
  refresh policy, drift caught in CI.

## References

- crtx v0.1 spec — envelope: <https://spec.hop.top/crtx/v0.1/envelope.schema.json>;
  events: <https://spec.hop.top/crtx/v0.1/events.schema.json>
- Producer ADR — [`crtx-producer.md`](crtx-producer.md)
- kit/bus adoption ADR — [`kit-bus-adoption.md`](kit-bus-adoption.md)
- stem (JSONLStore convention): <https://github.com/hop-top/poly-stem>
- vein (reader discipline): <https://github.com/hop-top/vein>
- Event catalog (inbox_message_* shapes) — [`spec/events.yaml`](../../spec/events.yaml)
- Guardrails escalation stub — [`escalate.go`](../../internal/hookpkg/builtins/guardrails/escalate.go)
- Stories — [`US-0003`](../stories/US-0003-drift-signals-trigger-inbox-write.md),
  [`US-0005`](../stories/US-0005-inbox-triage-and-promote.md),
  [`US-0007`](../stories/US-0007-bus-events-consumable-by-other-tools.md)
