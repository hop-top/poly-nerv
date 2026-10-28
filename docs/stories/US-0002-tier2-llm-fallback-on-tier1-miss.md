# US-0002: Tier 2 LLM fallback catches Tier 1 misses

**Persona:** Solo Developer, AI Agent
**Priority:** P0

## Story

As a solo developer, I want the open-questions hook to
catch unconventional question formats (not just `Q1./Q2.`
or "Want me to X?") via an LLM fallback when regex matchers
miss, so the inbox isn't blind to legitimate questions
phrased outside my pre-coded patterns.

## Acceptance Criteria

- [ ] Given an assistant turn that LOOKS questiony (ends
      with `?`, contains "confirm" / "pick" / "decide") AND
      Tier 1 regex matchers find ZERO questions, when the
      hook runs, then it invokes the configured LLM via
      `kit/llm` and asks for structured extraction
- [ ] Given LLM Tier 2 extracts ≥ 1 question, when the user's
      next turn doesn't address it, then an inbox entry is
      written AND the LLM-detected pattern is logged to
      `$XDG_STATE_HOME/nerv/open-questions/improvement-gaps.jsonl`
      with `{turn_text, llm_extracted_questions, suggested_regex}`
- [ ] Given LLM Tier 2 is unavailable (offline, budget
      exhausted), when Tier 1 finds nothing, then the hook
      logs a warning to the event log and continues without
      writing an inbox entry — fail-open, not fail-closed
- [ ] Given config `hooks.builtin.open-questions.llm.enabled
      == false`, when Tier 1 misses, then NO LLM call is made
      and NO improvement-gap entry is written
- [ ] Given the same turn is processed twice (replay), when
      Tier 2 ran the first time, then the LLM extraction is
      cached and not re-invoked

## Notes

- Decision: ship Tier 1 + LLM Tier 2 from day one.
  Tier 1 alone is too narrow per corpus analysis.
- Improvement-gap log feeds a future workflow (manual or
  automated) that proposes new Tier 1 regex matchers from
  recurring LLM catches.
