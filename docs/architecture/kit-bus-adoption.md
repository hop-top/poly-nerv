# ADR: kit/bus adoption — foundation

Last updated: 2026-04-26
Author: $USER
Status: Proposed

## Context

nervd today writes hook outcomes to a date-rotated JSONL eventlog
(`internal/eventlog/jsonl.go`). Built-in hooks (none yet — proposed
in the builtins-registry ADR) need to emit lifecycle events
that other in-process subscribers and external tools can consume
without tailing files or polling.

Driving need: the open-questions hook blocks on phase-1 inbox events. US-0007
specifies three subscriber modes (in-process, file-tail, socket)
and acceptance latencies (50 ms in-process, 100 ms jsonl).

`hop.top/kit/go/runtime/bus` already provides a Bus interface with
pluggable adapters (memory, sqlite, network), a JSONLSink, MQTT-
style topic matching (`*` / `#`), sync + async handlers, and Close
semantics. We adopt it rather than build a nerv-specific dispatcher.

## Decision drivers

- Must not regress fail-open: bus failure ≠ dispatch failure.
- Must be hot-path safe: default adapter zero-overhead per Publish.
- Must reuse kit/bus — do NOT fork or wrap it for nerv.
- Must keep the eventlog audit trail (compliance + replay).
- Must let external tools (ctxt, tlc, agent harnesses) subscribe.
- Must encode event-naming policy (container vs message-level):
  `<container>_<verb>` vs `<container>_<item>_<verb>`.

## Decisions

### 1. Adapter selection — default memory; configurable via `bus:` section

nervd defaults to `MemoryAdapter` (in-process pub/sub, no I/O on the
hot path). Operators opt into other adapters via a top-level `bus:`
section in `hooks.yaml`. Justification: bus config is config — the
existing layered loader (`internal/config/loader.go`) already merges
`/etc/nerv/hooks.yaml` → `~/.config/nerv/hooks.yaml` → `.nerv/hooks.yaml`.
Adding a sibling file would double the path-walk and confuse
operators. Adapter selection rides the same precedence as hook
defaults.

YAML schema additions (extends `spec/hooks.schema.json`):

```yaml
version: "1.0-draft"

bus:
  adapter: memory                    # memory | jsonl | sqlite | network
  sinks:                             # subscribers attached at startup
    - kind: jsonl
      path: $XDG_STATE_HOME/nerv/events.jsonl
      rotate:
        max_bytes: 50_000_000
        max_files: 7
    - kind: stdout                   # debug only; off by default
      enabled: false
  network:                           # only when adapter == network
    listen: ""                       # empty = client-only
    connect: []                      # peer addresses
  filters:                           # narrows what hits the sink
    topic_allow: ["inbox_message_*", "nerv.dispatch.#"]
    topic_deny:  []

defaults: { ... }
hooks:    [ ... ]
```

Notes:

- `adapter: memory` + `sinks: [jsonl]` is the recommended config:
  hot path stays in-memory; jsonl writes happen on the async fan-out.
- `adapter: jsonl` is shorthand for `memory` + a single jsonl sink.
- `adapter: sqlite` and `adapter: network` are listed for forward-
  compat — phase-1 implements `memory` only; the bus config schema
  may ship the full enum.
- `bus:` is OPTIONAL. Empty / missing → memory adapter, no sinks.

Phase-1 `bus.HooksConfigBus` struct:

```go
type HooksConfigBus struct {
    Adapter string         `yaml:"adapter"`            // default "memory"
    Sinks   []SinkConfig   `yaml:"sinks"`
    Network *NetworkConfig `yaml:"network,omitempty"`
    Filters *FilterConfig  `yaml:"filters,omitempty"`
}

type SinkConfig struct {
    Kind    string       `yaml:"kind"`                  // jsonl | stdout
    Path    string       `yaml:"path,omitempty"`        // jsonl only
    Rotate  *RotateCfg   `yaml:"rotate,omitempty"`
    Enabled *bool        `yaml:"enabled,omitempty"`
}

type RotateCfg struct {
    MaxBytes int64 `yaml:"max_bytes"`                   // 0 = no rotation
    MaxFiles int   `yaml:"max_files"`                   // 0 = keep all
}
```

### 2. Bus initialization in nervd — startup, daemon main, Runner-threaded

The bus is constructed once in `cmd/nervd/main.go` after config load
and threaded through to the Runner. The current `main.go` is a stub
that reads stdin and writes `{"action":"allow"}`; that scaffold gets
the bus init step added next to config load (already implied by
`_ "hop.top/kit/config"` import).

Construction order at startup:

1. `flag.Parse(); startPprof()`
2. `cfg, err := config.Load()` — layered hooks.yaml load
3. `b := buildBus(cfg.Bus)` — concrete `bus.Bus`
4. `attachSinks(b, cfg.Bus.Sinks)` — register JSONLSink etc.
5. `runner := dispatch.NewRunner()`
6. `dispatcher := dispatch.New(cfg.Cached(), logger, b, runner)`
7. `defer dispatcher.Close(ctx)` — flushes async handlers, closes sinks

`buildBus` is a thin factory — no business logic — that maps
`cfg.Bus.Adapter` onto kit/bus options:

```go
func buildBus(c HooksConfigBus) bus.Bus {
    switch c.Adapter {
    case "", "memory", "jsonl":
        return bus.New()                            // MemoryAdapter
    case "sqlite":
        return bus.New(bus.WithAdapter(bus.NewSQLiteAdapter(...)))
    case "network":
        return bus.New(bus.WithNetwork(c.Network.Connect...))
    default:
        log.Printf("bus: unknown adapter %q; falling back to memory",
            c.Adapter)
        return bus.New()
    }
}
```

Failure mode: if adapter init errors, log + fall back to
`MemoryAdapter` with no sinks. Bus is a side channel; nervd MUST
keep dispatching. Aligns with `spec/failure-modes.md`.

### 3. Bus instance threading — through dispatch into BuiltinFunc

Phase 1 wires the bus into `dispatch.Dispatcher` and a new
`BuiltinRuntimeCfg` consumed by built-in hooks (the builtins-registry ADR
defines `BuiltinFunc`).

```go
// internal/dispatch/dispatch.go
type Dispatcher struct {
    config *config.CachedConfig
    logger eventlog.Logger
    bus    bus.Bus            // NEW
    runner *Runner
}

func New(cfg *config.CachedConfig, lg eventlog.Logger,
         b bus.Bus, r *Runner) *Dispatcher { ... }

// internal/dispatch/run.go (extension, not yet implemented)
type BuiltinRuntimeCfg struct {
    Bus     bus.Bus
    Env     *envelope.Envelope
    Logger  eventlog.Logger
    Cwd     string
    Session string
}

type BuiltinFunc func(ctx context.Context,
                      cfg BuiltinRuntimeCfg) (merger.Decision, error)
```

`Runner.runOne` already forks subprocesses for exec hooks; built-in
hooks bypass exec and call `BuiltinFunc` directly with a populated
`BuiltinRuntimeCfg`. The bus reference is stable for the lifetime
of the daemon; built-ins do NOT close it.

Why a struct (not naked `bus.Bus` arg)? Forward-compat: future
fields (Cwd already in envelope, but ctxt client, tlc client, kit
xdg paths) plug in without changing every built-in's signature.

External (subprocess) hooks do NOT receive the bus — they get
envelope on stdin per `spec/handler-contract.md`. If a subprocess
hook needs to publish, it talks to nervd over the daemon socket
(phase 3 network adapter) or appends to the jsonl sink directly
(file-tail mode).

### 4. Canonical event schema versioning — per-payload `schema_version`

Each payload carries a `schema_version` string field (semver-ish:
`"1.0"`, `"1.1"`, `"2.0"`). Per-event, NOT global, NOT in the
envelope `Event`.

Rationale:

- Global (`bus.Event` envelope field): forces lockstep across all
  events. Adding a field to one payload bumps every consumer.
- Per-event in catalog YAML only: catalog drift vs runtime drift
  cannot be reconciled by subscribers without out-of-band schema
  fetching.
- Per-payload field: subscribers introspect the line, branch on
  version, ignore unknown fields. JSON-friendly. Cheap.

Version bump rules:

- Additive field → MINOR (`1.0` → `1.1`); subscribers ignore
  unknown fields.
- Field rename / removal / type change → MAJOR (`1.x` → `2.0`);
  publisher emits old + new for one release window.

Catalog (`spec/events.yaml` or `spec/bus-events.yaml`) records the
canonical schema version per event so `nerv lint` can validate
config + handlers.

### 5. Event schema additions — `inbox_message_*` payloads

Decision: add to existing `spec/events.yaml`, NOT a new
`spec/bus-events.yaml`. Reasons:

- One canonical event registry simplifies codegen
  (`cmd/nerv-codegen` reads one source).
- Bus events and adapter-translated CLI events share the same
  envelope shape; splitting catalogs invites drift.
- The 30-event catalog already mixes session/tool/agent/task
  categories — `bus` becomes a new `category` value, not a new
  file.

YAML additions (drop-in to `spec/events.yaml` under an
`# ── Inbox lifecycle (bus events) ──` block):

```yaml
- name: inbox_message_added
  category: bus
  description: A new entry arrived in an inbox container.
  direction: outbound
  blocking: false
  payload:
    - field: schema_version
      type: string
      example: "1.0"
    - field: msg_id
      type: string
    - field: agent_id
      type: string
    - field: inbox_id
      type: string
    - field: source_session_id
      type: string
    - field: kind
      type: string
      example: "open-question"
    - field: ts
      type: string
      format: rfc3339
    - field: payload_ref
      type: string
      description: opaque pointer (file path, ctxt id, etc.)

- name: inbox_message_updated
  category: bus
  description: An existing inbox entry was edited in-place.
  direction: outbound
  blocking: false
  payload:
    - field: schema_version
      type: string
    - field: msg_id
      type: string
    - field: fields_changed
      type: array
    - field: ts
      type: string
      format: rfc3339

- name: inbox_message_promoted
  category: bus
  description: An inbox entry was promoted to a downstream system.
  direction: outbound
  blocking: false
  payload:
    - field: schema_version
      type: string
    - field: msg_id
      type: string
    - field: target
      type: string
      enum: ["tlc-task", "ctxt-object", "other"]
    - field: target_id
      type: string
    - field: ts
      type: string
      format: rfc3339

- name: inbox_message_dropped
  category: bus
  description: An inbox entry was removed without promotion.
  direction: outbound
  blocking: false
  payload:
    - field: schema_version
      type: string
    - field: msg_id
      type: string
    - field: reason
      type: string
      enum: ["user-deleted", "ttl-expired", "duplicate"]
    - field: ts
      type: string
      format: rfc3339
```

Bus topic is the event `name` verbatim (snake_case). Topic-pattern
subscribers use `inbox_message_*` to capture all four.

### 6. NOT in scope (RESERVED) — `inbox_created`, `inbox_destroyed`

Reserved for inbox CONTAINER lifecycle:

- `inbox_created` — a new inbox container exists (e.g. fresh agent
  spawned, new project inbox dir created).
- `inbox_destroyed` — container removed (agent retired, inbox
  archived).

DO NOT publish these for entries arriving in the inbox. That is
`inbox_message_added`. Confusing the two breaks the canonical
naming rule:

- `<container>_<verb>` for container-lifecycle events.
- `<container>_<item>_<verb>` for instance / item events.

Examples elsewhere in the ecosystem:

| Container event       | Item event                                   |
|-----------------------|----------------------------------------------|
| `tlc_track_created`   | `tlc_task_claimed`, `tlc_task_completed`     |
| `aps_profile_created` | `aps_profile_updated`                        |
| `inbox_created`       | `inbox_message_added`, `inbox_message_dropped`|

Phase-1 publishes ONLY the four `inbox_message_*` events. Container
events ship with the inbox-management track (TBD; not this ADR).

Rationale: this rule is the ecosystem-wide canonical naming policy
(2026-04-26). ADR locks it in for nerv.

### 7. jsonl sink — replaces direct eventlog write at the dispatcher seam

Today: `Dispatcher.Dispatch` calls `d.logger.Write(env, merged)`
synchronously after merge. Phase-2 inverts this: the
dispatcher PUBLISHES a `nerv.dispatch.completed` event onto the bus;
`eventlog` becomes a SUBSCRIBER (a `Sink`) — not a separate writer.

Phase-1 scope (this ADR + JSONL sink):

- Add `JSONLSink` from kit/bus to the daemon, attached at startup
  via `cfg.Bus.Sinks`.
- Default path: `$XDG_STATE_HOME/nerv/events.jsonl` (single file,
  not date-rotated like the current eventlog). Rationale:
  consumers tail one file; rotation handled by `rotate:` config or
  external `logrotate`.
- Existing `eventlog.JSONLLogger` keeps writing for now (parallel
  paths). Phase-2 deprecates the direct call.
- Filters (`bus.filters.topic_allow`) restrict what hits the file
  to keep size bounded — default allows
  `["inbox_message_*", "nerv.dispatch.#"]`.

Rotation behavior:

- `rotate.max_bytes > 0`: on Drain, if current file exceeds limit,
  rename to `events.jsonl.1` (and shift prior backups down to
  `max_files`).
- `rotate.max_bytes == 0`: append forever (operator runs
  logrotate).
- Rotation is best-effort, lock-free per-process (nervd is single-
  writer in daemon mode).

Cross-process append safety: jsonl sink uses
`os.O_APPEND | os.O_CREATE` (kit/bus already does this in
`NewJSONLSinkFile`); short writes on POSIX append are atomic up to
PIPE_BUF (4 KiB) — fine for our event size envelope.

### 8. Subscriber documentation — three modes, worked examples

Three subscribe modes, each with a worked example.

#### Mode A: in-process (memory adapter, in-binary subscriber)

Use case: built-in hooks reacting to other built-in hooks within
the same nervd process. Lowest latency (< 1 ms typical).

```go
// inside a built-in hook's init/registration
unsub := runtimeCfg.Bus.Subscribe("inbox_message_added",
    func(ctx context.Context, e bus.Event) error {
        p, ok := e.Payload.(InboxMessageAddedPayload)
        if !ok { return nil }
        log.Printf("noticed new inbox msg %s", p.MsgID)
        return nil
    })
defer unsub()
```

#### Mode B: file-tail (jsonl sink, external tools)

Use case: ctxt, tlc, agent harness, observability tools running
out-of-process. Latency: bounded by sink Drain (~ ms) + reader's
poll interval.

```bash
# tail and parse with jq
tail -F "$XDG_STATE_HOME/nerv/events.jsonl" |
  jq -c 'select(.topic | startswith("inbox_message_"))'
```

```go
// external Go tool, polling tail
f, _ := tail.TailFile(path, tail.Config{Follow: true, ReOpen: true})
for line := range f.Lines {
    var ev jsonlEvent
    if err := json.Unmarshal([]byte(line.Text), &ev); err != nil { continue }
    if !strings.HasPrefix(ev.Topic, "inbox_message_") { continue }
    // ... handle
}
```

Schema of one jsonl line is the kit/bus `jsonlEvent` shape:
`{topic, source, timestamp, payload}`.

#### Mode C: socket (network adapter — phase 3)

Use case: cross-host subscribers, fan-out to other daemons. Phase
1 reserves config slot only (`bus.network`). Phase 3 implements
listener + auth + filters (kit/bus already provides
`network_auth.go`, `network_filter.go`).

```yaml
bus:
  adapter: network
  network:
    listen: "127.0.0.1:7745"
    connect: []
```

```go
// remote subscriber
b := bus.New(bus.WithNetwork("nervd-host:7745"))
defer b.Close(ctx)
b.SubscribeAsync("inbox_message_*", remoteHandler)
```

## Consequences

### Positive

- One canonical event surface (`spec/events.yaml`) for codegen,
  lint, replay, cassettes.
- External tools (ctxt, tlc, hop) integrate via documented sink
  modes — no scraping, no polling.
- Hot-path latency unchanged: memory adapter, async fan-out.
- Eventlog evolution path: phase-2 makes it a sink, not a special
  case.
- Forward-compat: sqlite + network adapters slot into the same
  config without code change.

### Negative

- Two parallel writers in phase-1 (existing JSONLLogger + new
  JSONLSink). Operators MAY see duplicate event records until
  phase-2 lands.
- Bus failure modes are silent by design (fail-open). Operators
  reading `events.jsonl` will not notice if the sink crashes mid-
  process; need a `bus.sink.errors` topic in a follow-up.
- `BuiltinRuntimeCfg` becomes the new "kitchen sink" for built-in
  hook deps. Risk of growing without bound; mitigate by reviewing
  every field add.

### Risks

- Rotation race in jsonl sink under multi-process write (nervd
  daemon + a fall-back stateless `nervd` invocation). Mitigation:
  daemon mode is single-writer; stateless mode skips the bus
  entirely (no daemon, no shared sink).
- Schema-version drift between catalog YAML and on-wire payload.
  Mitigation: `nerv lint` validates Go struct tags vs catalog at
  CI; codegen generates payload structs from catalog as truth.

## Implementation tasks (phase 1)

This ADR is the design baseline for:

- wire kit/bus into nervd startup (memory adapter default).
- canonical event schema in `spec/events.yaml`.
- bus config schema (this ADR's section 1).
- JSONL sink at `$XDG_STATE_HOME/nerv/events.jsonl`.

Phase-2 migrates `eventlog.Append` → `bus.Publish`.

## Diagrams

- [`kit-bus-adoption/dispatch-flow-v1.mmd`](kit-bus-adoption/dispatch-flow-v1.mmd)
  — bus instance flowing from daemon init through dispatch into
  `BuiltinFunc`.
- [`kit-bus-adoption/event-schema-v1.mmd`](kit-bus-adoption/event-schema-v1.mmd)
  — class diagram of the four `inbox_message_*` payloads with the
  reserved container events called out.

## References

- kit/bus source — `go/runtime/bus/` in hop-top/poly-kit
  (`bus.go`, `event.go`, `adapter.go`, `jsonlsink.go`).
- Event-naming policy — §6 above.
- Sibling story — [US-0007][us0007] (bus events consumable by
  other tools).

[us0007]: ../stories/US-0007-bus-events-consumable-by-other-tools.md
- Existing eventlog — [`internal/eventlog/jsonl.go`](../../internal/eventlog/jsonl.go).
- Existing dispatch — [`internal/dispatch/dispatch.go`](../../internal/dispatch/dispatch.go).
- Existing config loader — [`internal/config/loader.go`](../../internal/config/loader.go).
- Existing events catalog — [`spec/events.yaml`](../../spec/events.yaml).
- Failure-mode invariants — [`spec/failure-modes.md`](../../spec/failure-modes.md).
- nerv architecture — [`docs/architecture.md`](../architecture.md).

## Open questions (founder review)

1. Should `bus.adapter: jsonl` (single-file shorthand) be kept, or
   force operators to spell out `adapter: memory` + `sinks:`?
   Shorthand is friendlier; explicit is less magical.
2. JSONL sink default path — single file (`events.jsonl`) vs date-
   rotated (`events-YYYY-MM-DD.jsonl`, current eventlog convention)?
   ADR proposes single file with optional rotation; flips burden
   onto rotation config.
3. Phase-1 ships `memory` adapter only; should the YAML schema
   validate `adapter` enum strictly (reject `sqlite`/`network`)
   until phase-2/3, or accept-and-warn?
