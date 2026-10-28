# Persona: OSS Go Developer (nerv Extension)

**Primary Role:** External Go contributor sending patches to
hop-top/nerv. Cares about Go idioms, ergonomic test fixtures,
and clear extension points. Has zero context on internal IC
ops; needs the codebase to teach itself.

---

## Base Goals

- Land patches without internal coordination — README + ADRs
  are the source of truth.
- Run the full test suite locally in under a minute.
- Add a new feature (built-in, adapter, hook flag) by
  following an existing pattern, not by inventing one.

## Base Pain Points

- "How do I add X" answers buried in commit history; no
  living architecture doc.
- Build/test environment requires 8 setup commands; first PR
  takes a day before any code is written.
- Cross-package dependencies make incremental work risky —
  one change ripples through 12 files.

---

## nerv Interaction Patterns

### First contribution flow

```pseudocode
$ git clone https://github.com/hop-top/nerv
$ cd nerv
$ make test         // expects: green, < 60s
$ cat docs/architecture.md
// finds links to specific ADRs (built-ins-registry,
// wizard-onboarding, kit-bus-adoption) — knows where to start
```

### Adding a built-in (per Automation Builder ADR walkthrough)

```pseudocode
// 1. New package: internal/hookpkg/builtins/<name>/
// 2. Define Def, Handle, Config types
// 3. Register: append to internal/hookpkg/builtins/registry.go All
// 4. Mirror metadata: spec/builtins.yaml entry
// 5. Tests in package; integration in e2e/
// 6. PR
```

### Reading the bus event surface

```pseudocode
$ cat spec/bus-events.yaml  // canonical event schema
$ ls docs/architecture/kit-bus-adoption/
// ADR + diagrams describe naming rules + adapter selection
```

---

## User Stories

- As an OSS Go developer, I clone nerv and `make test` passes
  in under 60 seconds on a fresh checkout — no
  yaml-not-found, no missing system deps.
- As an OSS Go developer, I read `docs/architecture.md` and
  find direct links to the ADRs covering each subsystem so I
  don't have to grep for design rationale.
- As an OSS Go developer, I add a built-in by following the
  walkthrough in `builtins-registry.md` §7 and ship a PR
  without filing a "how do I?" issue.
- As an OSS Go developer, I publish bus events from my new
  built-in by following the naming rule
  (`<container>_<item>_<verb>`) documented in
  `kit-bus-adoption.md`, and `builtins.Validate()` catches
  any mismatch at daemon startup.
- As an OSS Go developer, my PR's CI runs nerv's e2e tests
  against the same xrr cassettes used locally — no
  surprise failures from environment drift.

---

## Success Metrics

- Time from `git clone` to green `make test`: < 60s.
- Time from "I want to add X" to PR open: < 4 hours for
  Go-fluent dev with a small built-in or hook.
- Zero internal-only docs required to land a typical PR.
- ADR coverage: every public package in `internal/`
  referenced by at least one ADR or has a `doc.go` block
  with the same intent.
