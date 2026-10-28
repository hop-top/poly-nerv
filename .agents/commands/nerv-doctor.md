---
description: "Check nerv installation health and offer fixes"
argument-hint: "[--fix] [--format <table|json>]"
allowed-tools: Bash(nerv:*)
---

# Diagnose the nerv dispatch pipeline

Invoke the `nerv` skill (Doctor + diagnostics section) and run every
read-only probe against the local install — daemon reachability,
`hooks.yaml` schema, `hooks.lock` consistency, adapter wiring,
capability-map compatibility, event-log writability. WARN/FAIL
probes can be auto- or interactively-fixed.

## Argument parsing

- `--fix`: apply known remediations without prompting. Without it,
  the command prompts before each fix.
- `--format <table|json>`: pass-through to `nerv doctor`. The
  command always runs `--format json` internally for parsing, then
  reformats per the user's choice on display.
- All other flags pass through unchanged (`--verbose`, etc.).

## Execution

1. Run `nerv doctor --format json` and capture the structured probe
   results.
2. Render a human-readable summary: one line per probe, status
   colored (PASS/WARN/FAIL), and the failure detail inline.
3. For each WARN/FAIL probe with a known fix:
   - With `--fix`: run the remediation command and report.
   - Without `--fix`: prompt
     `"<probe> failed — apply fix \`<command>\`? [y/n]"`. Run only
     on `y`.
4. After applying any fixes, re-run `nerv doctor` to confirm the
   probes now pass; surface the new status.
5. If a WARN/FAIL has no known fix, surface the raw detail and
   suggest `nerv doctor --verbose`.

## Known fixes

| Probe failure | Fix command |
|---|---|
| Daemon stopped / socket unreachable | `nerv daemon start` |
| Adapter missing for `<cli>` | `nerv install add <cli>` |
| `hooks.lock` SHA mismatch | `nerv add --refresh <hook>` (warn if the flag is not yet wired) |
| Stale `hooks.yaml` (daemon hasn't reloaded) | `nerv daemon reload` |

## Related skill

See `.agents/skills/nerv/SKILL.md` (Doctor + diagnostics section)
for the full probe list, fail-open semantics, and the relationship
between `nerv doctor` and `nerv status`.
