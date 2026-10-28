# Vendored crtx v0.1 schemas

`envelope.schema.json` and `events.schema.json` are verbatim copies of
the crtx v0.1 spec schemas, vendored so tests validate offline.

- Upstream origin: the spec-crtx repository
  (<https://github.com/hop-top/spec-crtx>), `specs/v0.1/`.
- Canonical URLs: <https://spec.hop.top/crtx/v0.1/envelope.schema.json>,
  <https://spec.hop.top/crtx/v0.1/events.schema.json>.
- Version: v0.1 (Draft). Refresh these copies whenever upstream tags a
  new v0.1 revision; the schema is the contract of record (prose loses
  on conflict, per the spec's own validation section).

`examples/` vendors a subset of the spec's published examples for the
same version (envelope examples plus every `events/` payload example)
and follows the same refresh policy. The dispatch/injection envelope
examples are not vendored; the child-linkage fields they exercise
(`dispatched_from`, `injected_turns`) are covered by locally built
fixtures in the interop tests instead.
