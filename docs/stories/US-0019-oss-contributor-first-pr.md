# US-0019: OSS contributor's first PR path

**Persona:** OSS Go Developer
**Priority:** P2

## Story

As an OSS Go developer landing my first nerv PR (e.g. a new
built-in), I want the codebase to teach itself: clear ADRs,
a `make test` that just works, and a single walkthrough I
can follow without filing a "how do I?" issue.

## Acceptance Criteria

- [ ] Given I clone nerv fresh, when I run `make test`, then
      the suite passes in <60s on a development laptop AND
      produces no missing-dep errors
- [ ] Given I read `docs/architecture.md`, when I look for
      a subsystem (built-ins, wizard, bus), then there is a
      direct link to a dedicated ADR explaining its design
- [ ] Given I follow `builtins-registry.md` §7's
      walkthrough to add a built-in, when my PR is opened,
      then CI runs the same xrr cassettes I ran locally —
      no environment drift
- [ ] Given my new built-in publishes a bus event, when
      `builtins.Validate()` runs at daemon startup, then
      it checks the event name against the canonical rule
      from `kit-bus-adoption.md` AND fails fast on
      malformed names
- [ ] Given my PR adds a new package under `internal/`,
      when reviewers check coverage, then either an ADR
      references the package OR the package has a `doc.go`
      with intent + usage

## Notes

- This is a meta-story that the build/test/docs surface
  must support, more than a single feature. Useful as a
  smoke-test target during phase 2 hardening.
