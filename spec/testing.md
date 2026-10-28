# Testing Conventions

> Test strategy for nerv core, adapters, and SDKs.
> Fixture substrate: hop-top/xrr (unified cassette system).

## Test Backbone

| Language   | Framework                                            |
|------------|------------------------------------------------------|
| Go         | stdlib `testing` + `testify/require` + `pgregory.net/rapid` |
| TypeScript | vitest                                               |
| Python     | pytest                                               |
| Shell      | bats-core                                            |

## 6 Test Layers

### 1. Unit

- Scope: per-function, mocked deps, fast
- Target: all `internal/*` packages, envelope parsing, matcher
  engine, decision merger
- Speed: <1s per test; entire unit suite <30s
- Deps: zero external (no SQLite, no filesystem, no network)

### 2. Integration

- Scope: multi-package, real deps (SQLite, filesystem), no network
- Target: config loader + hook registry, event log persistence,
  adapter install flow
- Deps: real SQLite via `kit/sqlstore`, real filesystem, temp dirs
- Cleanup: `t.TempDir()` for isolation

### 3. E2E

- Scope: full binary, real CLI stubs, Docker containers
- Target: `nerv` binary end-to-end, adapter smoke tests per CLI
- Deps: Docker for CLI stub containers; built `nerv` binary
- Pattern: spawn `nerv` subprocess, send envelope via socket/stdin,
  assert decision + event log entry

### 4. SDK

- Scope: per-language SDK tests against mock nervd
- Target: Go/TS/Python SDK client libraries
- Pattern: start mock nervd (in-process or subprocess), SDK sends
  envelope, assert response matches expected decision
- Isolation: each test gets fresh mock server instance

### 5. Property

- Scope: fuzz/generative testing for edge cases
- Tools: `pgregory.net/rapid` (Go), hypothesis (Python)
- Target: envelope parsing (arbitrary JSON), matcher edge cases
  (regex boundary conditions), merge semantics (arbitrary
  decision combinations)
- Invariants:
  - block always wins over rewrite/warn/allow
  - rewrite composition is associative
  - warn messages never lost
  - error (exit >=4) from a hook is treated as `allow` for that
    hook's contribution only; does not override other hooks' decisions

### 6. Performance

- Scope: benchmarks for hot dispatch path
- Path: envelope parse -> match -> run -> merge -> reply
- Tools: Go `testing.B`, vitest bench
- Targets:
  - Envelope parse: <100us for 1KB payload
  - Matcher eval: <50us for 10-rule hooks.yaml
  - Full dispatch (no handler exec): <1ms
- CI: benchmark comparison on PR; regression = CI warning

## Coverage Targets

| Scope                    | Target | Notes                            |
|--------------------------|--------|----------------------------------|
| `internal/*`             | 100%   | Core logic; no exceptions        |
| `adapters/internal/*`    | 100%   | Shared adapter utilities         |
| `adapters/<cli>/*`       | 80%    | Per-CLI adapter code             |
| Regression tests         | —      | Mandatory from v1.1 onward       |

v1 is greenfield; no legacy regression burden. From v1.1, every
bug fix MUST include a regression test.

## Cassette Format

All recorded fixtures use xrr canonical format:
- JSON encoding
- Deterministic key ordering
- Redacted secrets (API keys, tokens, session IDs replaced with
  `REDACTED`)
- Stored in `testdata/cassettes/` per package

## Test Naming

| Language   | Convention                                           |
|------------|------------------------------------------------------|
| Go         | `Test<Unit>_<Scenario>` (e.g., `TestMerger_BlockWins`) |
| TypeScript | `describe('<unit>', () => { it('<scenario>') })`     |
| Python     | `test_<unit>_<scenario>` / `class Test<Unit>:`       |
| Shell      | `@test "<unit>: <scenario>"`                         |

## Test Organization

```
nerv/
  internal/
    dispatch/
      dispatch_test.go        # unit
      dispatch_bench_test.go  # performance
      testdata/cassettes/     # xrr fixtures
  adapters/
    claude/
      claude_test.go          # unit + integration
  test/
    e2e/                      # E2E tests
    sdk/                      # SDK tests
    property/                 # property tests
```

## CI Integration

- Unit + integration: run on every PR, every push to main
- E2E: run on PR merge to main (Docker required)
- SDK: run on PR when `sdk/` files changed
- Property: run nightly (longer execution time)
- Performance: run on PR; compare against baseline; warn on
  regression >10%
