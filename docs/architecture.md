# Architecture — nerv

Last updated: 2026-04-26
Author: $USER

## Purpose

`hop.top/nerv` is the Universal Hooks Protocol — a daemon
dispatcher plus envelope protocol that unifies hook event
handling across AI coding CLIs (Claude Code, Gemini CLI,
Codex CLI, OpenCode, Aider, Goose, Vibe, OpenHands).

Hook authors write a single handler. nerv's adapters translate
between each CLI's native event model and a shared envelope,
so the same hook runs anywhere.

## Context

The AI coding CLI landscape is fragmented: each tool has its
own hook system, event names, payload shapes, and decision
formats. Without nerv, hook authors maintain N implementations
for N CLIs and synchronise them by hand.

nerv inverts the relationship. Each CLI gets a thin adapter
that owns the translation; the rest of the system (dispatch,
match, redaction, merge, eventlog) is CLI-agnostic.

Cross-references:

- [kit](https://github.com/hop-top/kit/blob/main/docs/architecture.md) — CLI substrate; provides `core/uxp` adapter base, XDG paths, output rendering
- [wsm](https://github.com/hop-top/wsm/blob/main/docs/architecture.md) — workspace state; nerv rule engine reads workspace events for access/scope evaluation
- [ctxt](https://github.com/hop-top/ctxt/blob/main/docs/architecture.md) — knowledge graph; consumes derived events (MemoryRead, MemoryWrite)
- [hop](https://github.com/hop-top/hop/blob/main/docs/architecture.md) — orchestrator; spawns and monitors nervd

## Containers

| Container | Path | Role |
|---|---|---|
| `nervd` daemon | `cmd/nervd/` | Stateless hot-path dispatcher; auto-detects CLI; fail-open invariant |
| `nerv` CLI | `cmd/nerv/` | User-facing toolchain (hook add/remove, doctor, lint, test, tail, replay, status, upgrade, daemon start/stop) |
| Adapters (8) | `adapters/{claude,codex,gemini,opencode,aider,goose,vibe,openhands}/` | Per-CLI ToEnvelope + FromDecision + capabilities.yaml |
| Specs | `spec/` | Envelope schema, events catalog, capabilities schema, hooks schema, redaction patterns |

## Components

### Dispatch engine (`internal/dispatch/`)

Five-phase pipeline:

1. **Config load** — layered: `/etc/nerv/hooks.yaml` → `~/.config/nerv/hooks.yaml` → `.nerv/hooks.yaml` → env (future)
2. **Match** — filter handlers by event type and scope rules (`match.go`)
3. **Run** — execute matched handlers (`run.go`); subprocess fork per event in stateless mode, Unix socket in daemon mode
4. **Merge** — combine multiple decisions per merge precedence (`internal/merger/merger.go`): `block > rewrite > warn > allow`
5. **Eventlog** — write decision + timing as JSONL for audit and replay (`internal/eventlog/`)

The fail-open invariant means: if nerv itself errors (config corrupt, handler crash, socket broken), the CLI is never blocked. Errors log; events pass through.

### Adapters (`adapters/`)

Eight adapters today. Full per-adapter event coverage matrix:
[`docs/capability-matrix.md`](capability-matrix.md) (✓ native / ~
synthesized / - uncovered, 8 adapters × 26 events). Quick summary:

| CLI | Native events | Synthesised | Coverage |
|---|---:|---:|---:|
| Claude Code | 26 | 0 | 26/26 |
| Gemini CLI | 8 | 16 | 24/26 |
| OpenCode | 10 | 13 | 23/26 |
| Goose | 11 | 11 | 22/26 |
| OpenHands | 5 | 15 | 20/26 |
| Codex CLI | 3 | 12 | 15/26 |
| Vibe | 6 | 8 | 14/26 |
| Aider | 0 | 5 | 5/26 |

Each adapter implements `adapters/internal/adapter.go`:

- `ToEnvelope(nativeEvent) -> Envelope` — translate inbound
- `FromDecision(Decision) -> nativeReply` — translate outbound
- `capabilities.yaml` — declares native vs synthesised vs unsupported events

Synthesised events are inferred from CLI internals (MCP traffic, SSE streams, exit codes, diffs) when the CLI doesn't emit them natively.

### Redaction (`adapters/internal/redact/`)

17+ regex patterns from `spec/redaction.yaml` strip secrets
(API keys, tokens, env vars) from envelopes before any handler
sees them. Applied unconditionally in the adapter ToEnvelope
path.

### Hook installer (`internal/hookpkg/install.go`)

Manages `~/.nerv/hooks/` directory and a per-CLI lockfile.
`nerv hook add <handler>` installs and registers; `nerv hook
remove` uninstalls.

### Event log (`internal/eventlog/`)

Writes JSONL audit logs for every dispatch. Used by:
- `nerv tail` — live observation
- `nerv replay` — re-run past events through current handlers
- Conformance testing — record + replay cassettes

## Public surfaces

### Hook handler contract

Specified at [`spec/handler-contract.md`](../spec/handler-contract.md).

- **stdin:** nerv envelope (JSON)
- **stdout:** decision object (JSON)
- **exit code:**
  - `0` allow
  - `1` warn
  - `2` block
  - `3` rewrite
  - `≥4` error (fail-open: treated as allow + log)

Action MUST match exit code; on mismatch, exit code wins.

### Daemon socket protocol

Specified at [`spec/socket-protocol.md`](../spec/socket-protocol.md).

- **Path:** `$XDG_RUNTIME_DIR/nerv.sock` (or `$TMPDIR/nerv.sock` on macOS)
- **Wire format:** 4-byte big-endian uint32 length prefix + JSON payload
- **Max message:** 10 MiB
- **Handshake:** version negotiation

### Schemas

| Schema | Path | Purpose |
|---|---|---|
| Envelope | [`spec/envelope.schema.json`](../spec/envelope.schema.json) | `$id: https://nerv.hop.top/schemas/envelope/1.0-draft` — incoming event shape |
| Events catalog | [`spec/events.yaml`](../spec/events.yaml) | 30 events: SessionStart, UserPromptSubmit, PreToolUse, PostToolUse, PermissionRequest, SubagentStart, TaskCreated, InstructionsLoaded, MemoryRead, MemoryWrite, etc. |
| Capabilities | [`spec/capabilities.schema.json`](../spec/capabilities.schema.json) | Per-CLI native vs synthesised vs unsupported declarations |
| Hooks config | [`spec/hooks.schema.json`](../spec/hooks.schema.json) | User-authored hook definitions |
| Redaction patterns | [`spec/redaction.yaml`](../spec/redaction.yaml) | 17+ regex patterns for secret stripping |

### CLI surface

| Command | Purpose |
|---|---|
| `nerv hook add <handler>` | Register a hook |
| `nerv hook remove <handler>` | Unregister |
| `nerv doctor` | Validate config, check CLI versions |
| `nerv lint` | Validate hook + config files |
| `nerv test` | Run hook against cassette fixtures |
| `nerv tail` | Live event stream |
| `nerv replay <cassette>` | Re-run past events through current handlers |
| `nerv status` | Daemon + hook status |
| `nerv upgrade` | Print version + upgrade instructions (no self-update) |
| `nerv daemon start\|stop` | Daemon lifecycle |

## Integrations

| Component | Direction | How |
|---|---|---|
| AI coding CLIs | inbound | Each adapter embeds `nervd` as a subprocess (stateless: fork-per-event; daemon: Unix socket) |
| `hop.top/kit` | dependency | `core/uxp.Adapter` base; XDG paths; output rendering |
| `wsm` rule engine | reads from nerv eventlog | Workspace events feed access/scope rule eval |
| `ctxt` knowledge graph | derived events | MemoryRead/MemoryWrite feed the knowledge graph |
| Marketplace hooks (future) | HTTP/gRPC | Out-of-process handlers via marketplace-proxy |

## Data flow — single-event dispatch

1. CLI emits a native event (e.g. Claude `PostToolUse`)
2. CLI's adapter calls `ToEnvelope()` → nerv envelope
3. Adapter writes envelope JSON to nervd stdin (fork mode) or socket (daemon mode)
4. nervd config-loads → matches → runs handlers → merges decisions
5. nervd emits decision JSON
6. Adapter calls `FromDecision()` → CLI native reply
7. CLI applies decision (allow / block / rewrite / warn)
8. nervd writes JSONL audit event

## Build / test / release

### Makefile

| Target | Runs |
|---|---|
| `make build` | `bin/nerv bin/nervd` (CGO_ENABLED=0, ldflags version) |
| `make test` | Unit tests |
| `make test-bench` | Benchmarks |
| `make test-conformance` | Cassette replay (needs Docker CLIs) |
| `make lint` | golangci-lint |
| `make cover` | Coverage report (60% gate enforced) |
| `make install` | Install to GOBIN |

### CI workflows

- `ci.yaml` — test (coverage gate), lint, build, bench on push/PR
- `conformance-record.yaml` — record cassettes from live CLI runs
- `conformance-replay.yaml` — replay cassettes, validate adapter conformance
- `release.yaml` — goreleaser + tag

### Release process

[`docs/RELEASING.md`](RELEASING.md) is the source of truth.
Gate checklist: all conformance + scenario tests pass,
coverage > 60%, no perf regressions, `nerv doctor` all-green.

Version scheme: `v1.0.0-draft` today; `v1.0.0` planned after
Phase 11.

## Architecture decisions

Specs in `spec/` carry the load of formal architecture
documentation:

- [Handler contract](../spec/handler-contract.md) — stdin/stdout protocol; exit code semantics; merge order
- [Socket protocol](../spec/socket-protocol.md) — Unix socket handshake + version negotiation
- [Failure modes](../spec/failure-modes.md) — fail-open / fail-safe / fail-forward matrix
- [Testing](../spec/testing.md) — cassette + replay-mode validator (15 golden + 6 failure scenarios)

Living document: [`docs/research/mappings.md`](research/mappings.md) — event×CLI matrix maintained as new events / adapters land.

ADRs in `docs/architecture/`:

- [builtins-registry.md](architecture/builtins-registry.md) — registry pattern for built-in hooks
- [kit-bus-adoption.md](architecture/kit-bus-adoption.md) — phase-1 design for adopting `hop.top/kit/runtime/bus`
- [wizard-onboarding.md](architecture/wizard-onboarding.md) — `nerv init` wizard flow + builtin metadata schema

## Evolution

### Recent significant changes

- Phase 11 (CI / Docker / scenarios / conformance / release) approaching done
- Adapter ecosystem expanded 4 → 8: aider, goose, vibe, openhands added
- Phase 10: doctor, lint, test, tail, replay, status, upgrade
- Phase 9: hook add/remove, marketplace, agents-md starter
- Origin field on events (`feat(events): add origin field` — native|derived|extension)
- MemoryRead + MemoryWrite as first derived events
- ctxt integration
- kit bump from `v0.2.0-alpha.1` → `v0.3.2-patch.3` on remote

### Adopter status

8 CLI adapters in tree; coverage matrix in `docs/research/mappings.md` last verified 2026-04-17.

## Open questions

1. **Synthesis strategy.** Adapters infer PostToolUseFailure from exit codes / diffs (Codex, Gemini). Sufficient or do CLIs need explicit failure signals?
2. **Redaction completeness.** 17 regex patterns cover env vars, API keys, tokens. Hook-specific patterns (session IDs, project keys) may be missing.
3. **Merge priority.** `merger.go` hardcodes block > rewrite > warn > allow. Should this be a config knob?
4. **Daemon version negotiation.** `socket-protocol.md` mentions version negotiation but does not specify fallback when CLI v1.0 client connects to daemon v1.1.
5. **Synthesis cost.** Synthesising 15+ events (Gemini / OpenCode) requires hooking CLI internals (MCP, SSE, diffs). Long-term maintainable, or should synthesised events be officially "unsupported"?
6. **Kit coupling.** Local depended on `v0.2.0-alpha.1`; remote already bumped to `v0.3.2-patch.3`. Major version bump on kit stable?
