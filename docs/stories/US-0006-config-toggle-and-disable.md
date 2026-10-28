# US-0006: Toggle the open-questions hook on/off

**Persona:** Solo Developer, Platform Engineer
**Priority:** P0

## Story

As a solo developer (or platform engineer rolling out to a
team), I want to enable/disable the open-questions built-in
via plain `nerv config set` without uninstalling anything, and
I want layered config (system → user → project) to merge
correctly so per-project overrides work without clobbering
user-level settings.

## Acceptance Criteria

- [ ] Given the built-in is registered, when I run
      `nerv config set hooks.builtin.open-questions.enabled
      true`, then the next session has the hook firing on
      configured events
- [ ] Given the built-in is enabled, when I run
      `nerv config set hooks.builtin.open-questions.enabled
      false`, then the hook stays REGISTERED (visible in
      `nerv hook list` with status `disabled`) but does NOT
      fire on any event
- [ ] Given user layer sets `enabled: true` and project layer
      sets only `drift_keywords: [...]`, when config compiles,
      then BOTH `enabled: true` (from user) AND `drift_keywords`
      (from project) apply — field-level merge per OQ1
      resolution
- [ ] Given a config key has an invalid value (e.g.
      `state_ttl_hours: "forever"`), when the daemon loads
      config, then it logs an error AND uses the schema
      default — does NOT crash, does NOT silently apply garbage
- [ ] Given config changes via `nerv config set`, when the
      next session starts (or SIGHUP fires), then the new
      config takes effect without daemon restart

## Notes

- Per builtins-registry ADR OQ1 resolution: `BuiltinUserCfg` merges
  field-by-field across layers; `Extra map[string]any` also
  merges per-key.
- Per spec.md §"Toggle / config": all 7 config keys are
  documented; defaults are safe (off until stable).
