---
description: "Disable hooks, events, or hook×event bindings — thin wrapper over 'nerv disable / enable'"
argument-hint: "{hooks|events|bindings} <names...> [--no-confirm]"
allowed-tools: Bash(nerv:*)
---

# Disable hooks, events, or bindings

Invoke the `nerv disable` (or `nerv enable`) verb to flip dispatch
state at one of three granularities. The verb owns every write
(`.nerv/hooks.yaml`, `.nerv/disabled.yaml`); this slash command never
touches the filesystem directly.

## Argument parsing

- `{hooks|events|bindings}` (positional, REQUIRED): unit to disable.
- `<names...>` (positional, REQUIRED): one or more names. The verb
  matches them exactly against the current state.
  - For `hooks`: name from `.nerv/hooks.yaml`.
  - For `events`: canonical event name from
    `.agents/references/event-catalog.md`. The verb rejects unknown
    events against the catalog.
  - For `bindings`: `<hook>:<event>` pair per arg; same catalog
    validation on the event token.
- `--no-confirm`: skip the y/N prompt before persisting (required for
  non-TTY contexts; the verb refuses to proceed otherwise).
- `--enable`: invert — re-enable the named units. This slash command
  also accepts the inverse phrasing; see "Enable" below.

## Execution

1. Validate the sub-mode token is one of `hooks | events | bindings`.

2. Run:

   ```sh
   nerv disable <sub-mode> <names...> [--no-confirm]
   ```

   For the inverse:

   ```sh
   nerv enable <sub-mode> <names...> [--no-confirm]
   ```

3. Surface stdout. On non-zero exit, surface stderr verbatim so the
   user sees the "unknown event" / "no matching hook" / write error
   in full.

4. On success, suggest follow-ups:

   > Run `nerv daemon reload` (or `/nerv-doctor`) to apply the change
   > to a running daemon. Run `/nerv-tail` to confirm the disable
   > took effect on live traffic.

## Notes

- Idempotent: re-disabling an already-disabled name is a no-op. The
  verb prints "no <unit> changed (already disabled)" when nothing
  flips.
- The verb refuses to proceed without `--no-confirm` in a non-TTY
  context. From an agent loop, always pass `--no-confirm`.
- Daemon enforcement of the `disabled` flag lives in the dispatcher;
  out-of-band edits to `.nerv/hooks.yaml` or `.nerv/disabled.yaml`
  aren't picked up until `nerv daemon reload` (or SIGHUP).

## Related skill

See `.agents/skills/nerv/SKILL.md` — section "Lifecycle — Enable /
disable" for the conceptual model, and
`.agents/references/event-catalog.md` for the 36-entry canonical
event taxonomy used by the verb's validator.

## Related commands

- `/nerv-doctor` — verify daemon is running before reload
- `/nerv-tail` — watch live dispatches to confirm disables took effect
- `/nerv-create` — author a hook from intent (then disable it later
  without removing)
