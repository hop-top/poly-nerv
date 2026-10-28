# US-0014: Built-in hook dispatched in-process with fail-open

**Persona:** AI Agent, Automation Builder
**Priority:** P0

## Story

As an AI agent firing hook events (or as an automation
builder writing a built-in), I want built-ins to dispatch
in-process (no fork/exec) AND to share the same fail-open
guarantees as user hooks, so latency is low AND a buggy
built-in never breaks the event loop.

## Acceptance Criteria

- [ ] Given a built-in is registered AND enabled, when an
      event matching its `Events` fires, then `runBuiltin`
      is called instead of fork/exec — single-process,
      no subprocess
- [ ] Given the built-in's `Func` panics, when it executes,
      then `callBuiltinSafe`'s `defer recover()` returns a
      `Decision{Action: Allow}` AND logs the panic — does
      NOT crash nervd
- [ ] Given the built-in returns an error, when dispatch
      processes it, then the merger receives an `Allow`
      decision (fail-open) AND a warning is logged with
      the built-in's name + error message
- [ ] Given the built-in is `Enabled: false`, when an event
      fires, then `runBuiltin` short-circuits to `Allow`
      BEFORE calling `Func` — zero cost, builtin not invoked
- [ ] Given a built-in's `Func` exceeds a per-builtin
      timeout (configurable; default 5s), when timeout
      triggers, then dispatch returns `Allow` and logs the
      timeout — same contract as user hooks

## Notes

- Per builtins-registry ADR §4: same dispatch path as user hooks, routed
  via `HookEntry.Source == "builtin"`.
- Fail-open is a hard requirement; no built-in can become a
  blocker.
