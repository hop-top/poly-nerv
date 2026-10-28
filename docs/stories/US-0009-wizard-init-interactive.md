# US-0009: Interactive `nerv init` wizard

**Persona:** Solo Developer
**Priority:** P0

## Story

As a solo developer, when I install nerv for the first time,
I want to run `nerv init` and be walked through the available
built-in hooks (one prompt per built-in) with enough context
(description, risk level, default) to make an informed
choice — and have my answers persist to my user config file.

## Acceptance Criteria

- [ ] Given a fresh install (no `~/.config/nerv/wizard-state.yaml`),
      when I run `nerv init` in an interactive terminal, then I
      see one prompt screen per registered built-in with:
      `id`, multi-line `prompt`, `events`, `risk_level` (color-
      coded green/yellow/red via lipgloss), `default_enabled`,
      `requires_cli` summary
- [ ] Given a prompt, when I type `Y`, `n`, `skip`, or `quit`,
      then the wizard records the choice (Y/n) or defers
      (skip) or aborts without writes (quit)
- [ ] Given I make at least one Y choice, when the wizard
      completes, then `~/.config/nerv/hooks.yaml` is updated
      to include `builtin: { <id>: { enabled: true } }` under
      the user layer
- [ ] Given the wizard completes, then
      `~/.config/nerv/wizard-state.yaml` is updated with:
      `version`, `seen_at`, `detected_cli`, `seen_builtins`
- [ ] Given the wizard runs in a non-TTY context (CI, pipe),
      when no flags are passed, then it auto-applies defaults
      AND emits a JSON summary to stdout matching the schema
      in `wizard-onboarding.md` Decision 5
- [ ] Given I use `--from <preset.yaml>`, when the wizard
      runs, then it applies the preset map without prompting;
      `--yes` flag is mutually exclusive with `--from`
      (validates in `PreRunE`)

## Notes

- Builtin metadata read from `spec/builtins.yaml` — the
  wizard's source of truth for prompts.
- Validates `id` of each YAML entry against `builtins.All`
  via `builtins.Validate()` at startup; aborts loudly if
  YAML references a non-existent built-in (per OQ3 resolution).
