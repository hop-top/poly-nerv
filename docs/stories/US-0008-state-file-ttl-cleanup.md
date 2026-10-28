# US-0008: State file TTL cleanup

**Persona:** Solo Developer
**Priority:** P2

## Story

As a solo developer, I want stale state files (sessions
that were never flushed to inbox AND never resumed) to be
cleaned up automatically so my disk isn't a graveyard of
abandoned hook state.

## Acceptance Criteria

- [ ] Given a session state file exists and was last modified
      > `state_ttl_hours` (default 168 = 7 days) ago, when
      the daemon's periodic cleanup runs, then the state
      file is removed
- [ ] Given an inbox entry was promoted (`nerv inbox promote`),
      when promote completes, then the corresponding state
      file is removed immediately — without waiting for TTL
- [ ] Given a user runs `nerv inbox purge`, when it executes,
      then ALL state files under
      `$XDG_STATE_HOME/nerv/open-questions/` are removed
      regardless of age
- [ ] Given config `hooks.builtin.open-questions.state_ttl_hours`
      is set to a custom value (e.g. 24 = 1 day), when
      cleanup runs, then files older than the configured
      value are removed (config wins over default)
- [ ] Given a state file is removed, when the next session
      with the same id (rare — would be a replay) tries to
      read it, then the hook gracefully starts fresh — no
      crash on missing state

## Notes

- Per OQ4 resolution: 7-day default, configurable.
- Cleanup runs at daemon startup AND on SIGHUP (cheap pass
  through `glob + stat + unlink`).
- TTL applies to STATE files (per-session question tracking),
  NOT to inbox files (inbox files are user-owned, never
  auto-deleted by nerv).
