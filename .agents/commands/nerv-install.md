---
description: "Install nerv adapter for an AI CLI"
argument-hint: "[<cli>] [--all]"
allowed-tools: Bash(nerv:*)
---

# Wire a per-CLI adapter

Invoke the `nerv` skill (Adapters section) and write the per-CLI
config so the chosen AI CLI emits events into the nerv envelope
pipeline. The list of supported CLIs is derived dynamically from the
`adapters/` directory in the repo so the picker stays accurate as
new adapters land.

## Argument parsing

- First positional: CLI name (optional). Must be one of the values
  derived from `adapters/`. As of writing: `claude | codex | gemini |
  opencode | openhands | aider | goose | vibe`.
- `--all`: install every adapter the registry knows about.
- All other flags pass through unchanged (`--dry-run`, etc.).

## Execution

1. Enumerate supported CLIs by listing immediate subdirectories of
   `adapters/` (skip non-adapter dirs like `internal`). Treat this
   list as the source of truth for the picker — never hard-code it.
2. Branching on inputs:
   - `--all` set → invoke `nerv install add <cli>` for each entry in
     the list.
   - `<cli>` given → invoke `nerv install add <cli>`.
   - Neither → present a multi-select picker over the dynamic list
     and invoke `nerv install add <cli>` for each selection.
3. For each install: surface `nerv install add <cli>` stdout; on
   non-zero exit also surface stderr.
4. After all installs complete, run `nerv doctor` (no `--fix`) and
   surface the result so the user sees adapter wiring confirmed.

## Related skill

See `.agents/skills/nerv/SKILL.md` (Adapters section) for the
capability-map shape, the difference between native and synthesized
event coverage, and where each CLI's config lands on disk.
