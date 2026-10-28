# US-0011: Wizard skips built-ins with no matching CLI

**Persona:** Solo Developer
**Priority:** P1

## Story

As a solo developer who only has claude installed (not
codex or copilot), I don't want the wizard to prompt me
about built-ins that require a CLI I don't have — so I
don't have to read past irrelevant prompts.

## Acceptance Criteria

- [ ] Given a built-in declares `requires_cli: ["claude",
      "codex"]` AND `adapters.Detect()` finds only `claude`,
      when wizard processes it, then it is PROMPTED (claude
      is in the intersection)
- [ ] Given a built-in declares `requires_cli: ["codex"]`
      AND only `claude` is detected, when wizard processes
      it, then it is SKIPPED with a printed reason: `"  [skip]
      <id> — requires codex (not detected)"`
- [ ] Given a built-in declares `requires_cli: []` (empty =
      all CLIs), when wizard processes it, then it is always
      PROMPTED regardless of detected CLIs
- [ ] Given the user runs `nerv init --verbose`, when filtered
      skips occur, then each printed line includes the full
      reason (CLI requirements + detected CLIs)
- [ ] Given `wizard-state.yaml.detected_cli` is set on
      first run, when re-run, then the value is updated if
      the detected primary CLI changed (alpha-order winner
      among detected adapters per OQ4)

## Notes

- `SupportedCLIs()` returns alpha order — primary CLI is the
  first detected match in that order (no capability ranking).
- Filter check happens BEFORE the prompt loop; not visible
  to the user as a no-op turn.
