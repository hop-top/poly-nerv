# US-0010: Wizard idempotence and reset

**Persona:** Solo Developer, Platform Engineer
**Priority:** P1

## Story

As a solo developer, I want re-running `nerv init` to be
safe — only prompt me about built-ins I haven't seen before
— and as a platform engineer running rollouts, I want
`--reset` to re-prompt everything when team policy changes.

## Acceptance Criteria

- [ ] Given `wizard-state.yaml` lists built-in `open-questions`
      in `seen_builtins`, when I run `nerv init`, then the
      wizard skips it silently (no prompt) — it does NOT
      re-ask
- [ ] Given a NEW built-in (not in `seen_builtins`) is
      registered after a previous wizard run, when I run
      `nerv init` again, then ONLY the new built-in is
      prompted; previously-seen ones stay skipped
- [ ] Given I previously chose `skip` on built-in X, when I
      re-run `nerv init`, then X is RE-PROMPTED (skip ≠ seen
      per OQ1 resolution)
- [ ] Given I run `nerv init --reset`, when the wizard
      starts, then `wizard-state.yaml.seen_builtins` is
      cleared AND every built-in is prompted again
- [ ] Given `wizard-state.yaml` is corrupted (invalid YAML),
      when wizard reads it, then wizard treats it as
      missing AND prompts as if for the first time —
      does NOT crash

## Notes

- Skip means "ask me again next time" (deferred decision).
- Permanent disable lives in `hooks.builtin.<id>.enabled:
  false` set directly via `nerv config set` (or future
  `nerv hook disable`), NOT via wizard state.
