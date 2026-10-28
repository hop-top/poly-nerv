# US-0021: Guardrails rewrite commands to a safer variant where possible

**Persona:** Automation Builder, AI Agent
**Priority:** P1

## Story

As an agent running coding sessions where some footguns have a
strictly-safer alternative (e.g. `--force-with-lease` instead of
`--force`), I want guardrails to silently substitute the safer
form rather than blocking the operation. Blocking adds friction
when the intent is fine; rewriting preserves intent and prevents
the actual harm.

## Acceptance Criteria

- [ ] Given an adapter sends `git push --force origin <non-protected>`,
      when guardrails evaluates, then the decision is Rewrite with
      the rewritten command set to
      `git push --force-with-lease origin <non-protected>`
- [ ] Given the rewrite is applied, when the merger emits the final
      decision, then `Decision.Rewrite` is a map containing
      `command: "<rewritten>"` so the dispatch path can substitute
      the new command into the envelope before exec
- [ ] Given a user opts a rewrite-mode rule into block via
      `hooks.yaml → builtin.guardrails.rules.git-push-force.mode: block`,
      then the same envelope is Block instead of Rewrite
- [ ] Given a user opts a block-mode rule into rewrite, when the
      rule has at least one rewrite strategy declared, then the
      command is rewritten; if no strategy is declared, the rule
      falls back to Block (cannot rewrite without a strategy)
- [ ] Given the rewrite produces no change (regex matched but
      strategy was a no-op), then guardrails falls back to Block
      with reason "rewrite produced no change; falling back to
      block" — better to deny than silently allow

## Out of scope

- Rewrite for git-add-all (no safe automatic rewrite — must list
  files explicitly)
- Rewrite for rm rules (no safe automatic rewrite — paths are
  semantic)
- User-defined rewrite strategies (rule packs only ship strategies
  from the schema enum)

## Acceptance test path

- `matcher_test.go::TestMatcher_RewriteAppliesForcePush`
- `matcher_test.go::TestMatcher_OverrideMode`
- `rewrite_test.go::TestApplyRewrites_NoOpReportsFalse`

## Telemetry

- log line includes both original and rewritten command for
  audit / drift-detection downstream
