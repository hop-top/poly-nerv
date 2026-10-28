# US-0003: Drift signals trigger inbox writes

**Persona:** Solo Developer, AI Agent
**Priority:** P0

## Story

As a solo developer, I want any of the 5 drift signals to
flush my unanswered questions to the inbox, so I don't lose
them when context shifts mid-session.

## Acceptance Criteria

Each drift signal independently triggers an inbox flush IF
unanswered questions exist for the current session.

- [ ] **Assignment drift** — given unanswered questions, when
      the user runs `tlc task claim <new-id>` (different from
      `topic_anchor.task_id`), then an inbox entry is written
      with `trigger == "assignment_drift"`
- [ ] **Topic-keyword drift** — given unanswered questions,
      when the user's prompt contains a configured pivot
      keyword (default list per spec; user-configurable via
      `hooks.builtin.open-questions.drift_keywords`), then an
      inbox entry is written with `trigger == "topic_keyword"`
- [ ] **Subagent dispatch** — given unanswered questions, when
      a `Task` tool use fires (subagent dispatched), then an
      inbox entry is written with `trigger ==
      "subagent_dispatch"` AND the parent session record gains
      a child-session reference at the originating message_id
- [ ] **CWD change** — given unanswered questions, when a
      `CwdChanged` event fires (different repo), then an
      inbox entry is written with `trigger == "cwd_change"`
- [ ] **Session end / pre-compact** — given unanswered
      questions, when `SessionEnd` or `PreCompact` fires, then
      an inbox entry is written with `trigger == "session_end"`
      or `"pre_compact"` respectively

## Notes

- Each trigger publishes one `inbox_message_added` bus event.
- Multiple drift signals in rapid succession should NOT
  produce duplicate inboxes for the same questions; dedup via
  message-id stamping.
- Spec.md §"Drift detection" is the source of truth.
