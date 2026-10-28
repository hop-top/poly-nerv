---
description: "Replay a recorded event through the hook pipeline"
argument-hint: "[<event-id-or-prefix>] [--last N]"
allowed-tools: Bash(nerv:*)
---

# Replay a recorded event

Invoke the `nerv` skill (Tail + replay section) and re-dispatch one
or more recorded envelopes through the current hook config. Useful
for incident forensics ("what would today's hooks have decided about
yesterday's event?") and regression checks.

## Argument parsing

- First positional: event-id, or any unique prefix of one. `nerv
  replay` resolves prefixes against the event log.
- `--last N`: list the N most recent events for selection. Default
  N=10 when the flag is bare. Implies the discovery branch even when
  an event-id positional is supplied.
- `--dry-run`: pass-through; print decisions but don't merge.
- All other flags pass through unchanged.

## Execution

1. Discovery branch — when `<event-id>` is omitted, or when
   `--last N` is set:
   - Run `nerv tail --last N --format json` (default N=10).
   - Render the candidate list (event-id, event name, CLI,
     timestamp, payload snippet).
   - Present a multi-select picker; collect one or more event-ids.
2. Replay each selected event-id:

   ```sh
   nerv replay <event-id> [--dry-run]
   ```

3. Surface the merged decision per event (action, message, any
   rewrite payload). On non-zero exit also surface stderr.
4. When the user pastes an event-id prefix directly, hand it to
   `nerv replay` as-is — the verb resolves prefixes server-side and
   errors on ambiguity.

## Related skill

See `.agents/skills/nerv/SKILL.md` (Tail + replay section) for the
event log layout, prefix resolution rules, and the difference
between live tail and forensic replay.
