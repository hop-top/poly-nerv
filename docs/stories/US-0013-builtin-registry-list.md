# US-0013: List user hooks AND built-ins side-by-side

**Persona:** Platform Engineer, Solo Developer
**Priority:** P0

## Story

As a platform engineer (or solo dev verifying a setup),
I want `nerv hook list` to show user-installed hooks AND
built-ins in one view, with each tagged by source (user /
builtin) and status (enabled / disabled), so I can audit
the full event-routing tree at a glance.

## Acceptance Criteria

- [ ] Given user hooks exist (in `hooks.yaml`) AND built-ins
      are registered (in `builtins.All`), when I run `nerv
      hook list`, then BOTH appear in the output table
- [ ] Given the table format, when output renders, then
      every row has columns: NAME, SOURCE (user/builtin),
      EVENTS (count or comma-list), STATUS (enabled/disabled)
- [ ] Given a built-in has `Builtin[name].Enabled == false`,
      when listed, then its row shows `disabled` in STATUS
      and the row IS still present (not hidden)
- [ ] Given `--format json` is passed, when output renders,
      then the JSON array contains one object per hook
      matching the documented schema (source, name, events,
      status, schema_version for builtins)
- [ ] Given no user hooks AND no built-ins are enabled, when
      `nerv hook list` runs, then output is the header only
      (table) or `[]` (json) — exit 0, no error

## Notes

- Subcommand group `nerv hook` per OQ3 resolution; future
  subcommands (`nerv hook enable`, `nerv hook disable`, `nerv
  hook purge`) live under the same group.
- Mirrors `nerv daemon` precedent.
