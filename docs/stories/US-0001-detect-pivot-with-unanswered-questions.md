# US-0001: Detect pivot with unanswered open questions

**Persona:** Solo Developer, AI Agent
**Priority:** P0

## Story

As a solo developer (or AI agent reasoning about its own
output), when I emit a list of open questions to the user
and the user pivots without answering them, I want the
unanswered questions captured to an inbox so they aren't
lost in the next turn's context.

## Acceptance Criteria

- [ ] Given an assistant turn containing ≥ 1 numbered Q
      pattern (Q1./Q2., or "Open questions" header + numbered
      list, or `Want me to X or Y?`), when the user's next
      turn does NOT address them and contains a pivot keyword
      (e.g. "moving on", "let's switch"), then an inbox entry
      is written under
      `$XDG_STATE_HOME/nerv/inbox/<agent_id>/open-questions/`
- [ ] Given the same emit-then-pivot scenario, when the
      inbox entry is written, then a bus event
      `inbox_message_added` fires with `payload.kind ==
      "open_question"` and `payload.unanswered_count ==
      <total questions emitted>`
- [ ] Given user answers ALL emitted questions in the next
      turn (numbered response, letter choice, or affirmative),
      when no pivot occurs, then NO inbox entry is written
      and NO `inbox_message_added` event fires
- [ ] Given user answers PARTIAL (N of M questions),
      when the next turn occurs, then inbox entry is written
      with `unanswered_count == M - N`

## Notes

- Tier 1 detection is regex-based;
  Tier 2 LLM fallback closes the long tail per spec.md.
- Drift signals beyond keyword pivot are covered by US-0003
  (assignment, subagent, cwd, session-end).
