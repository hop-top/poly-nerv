---
description: "Run a hook handler against a synthetic envelope"
argument-hint: "[<hook>] [--event <name>] [--tool <name>] [--command <string>] [--all]"
allowed-tools: Bash(nerv:*)
---

# Test a hook against a synthetic envelope

Invoke the `nerv` skill (Lifecycle → Test before letting real
traffic in) and run a chosen hook handler against a synthetic
envelope. Nothing leaks back to a CLI; this is purely a dry-run of
the dispatch path against synthetic input.

## Argument parsing

- First positional: hook name (optional). Must match an entry in
  `.nerv/hooks.yaml`.
- `--event <name>`: canonical event name (one of the 36; see the
  reference). The slash command prompts for it if omitted.
- `--tool <name>`: tool name stamped into `payload.tool_name`
  (e.g. `Bash`, `Write`).
- `--command <string>`: command string stamped into
  `payload.tool_input.command` (for `Bash`-style tools).
- `--all`: run every hook with sample envelopes; no other input
  needed.

## Execution

1. Resolve the hook list — when `<hook>` is omitted and `--all` is
   not set:
   - Read `.nerv/hooks.yaml` directly, or fall back to
     `nerv lint --format json` for parsed output.
   - Present a single-select picker over the registered hook names.
2. Collect the envelope inputs:
   - **Event:** if `--event` is not set, present a picker over the
     36 canonical events. See `.agents/references/event-catalog.md`
     for the full list with payload shapes and per-CLI coverage.
   - **Tool / command:** for tool events (`PreToolUse`,
     `PostToolUse`, …) ask for `--tool` and, for shell tools,
     `--command`.
3. Invoke the verb:

   ```sh
   nerv test <hook> --event <name> [--tool <name>] [--command "<string>"]
   # or
   nerv test --all
   ```

4. Surface the merged decision: action, message, any rewrite, the
   handler's exit code. On non-zero exit also surface stderr.

## Related skill

See `.agents/skills/nerv/SKILL.md` (Lifecycle section) for the test
→ enable workflow, fail-open semantics, and the relationship
between `nerv test` and `nerv replay`.
