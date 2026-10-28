# US-0004: Subagent question state propagates to parent

**Persona:** AI Agent
**Priority:** P1

## Story

As an AI agent dispatched as a subagent, when I encounter
or answer a question that belongs to my parent session's
state, I want my action to update the PARENT'S state — not
write a separate inbox — so a single session-tree's open
questions are tracked coherently across parent + children.

## Acceptance Criteria

- [ ] Given a parent session has unanswered questions and
      dispatches a subagent at message-id X, when nerv records
      the dispatch, then the parent's session record gains a
      child-session reference `{child_session_id, dispatched_at_message_id == X}`
- [ ] Given a subagent answers (numbered, letter, or
      affirmative response) one or more of the parent's open
      questions, when the hook fires in subagent context,
      then the PARENT'S question state updates and NO
      separate inbox is written for the subagent
- [ ] Given a subagent's session ends without resolving any
      parent question, when `SubagentStop` fires, then the
      parent's question state is unchanged (no flush triggered
      by subagent end alone)
- [ ] Given the parent's `agent_id` (resolved via aps lookup
      to `CachedConfig.ActiveProfile`), when an inbox is
      written for an open-question event, then it lands at
      `$XDG_STATE_HOME/nerv/inbox/<parent_agent_id>/`, not
      under any subagent's identity
- [ ] Given a subagent itself emits open questions (rare),
      when those go unanswered AND the subagent terminates
      with parent still active, then questions are added to
      the PARENT'S unanswered-question list, not a separate
      bucket

## Notes

- Per OQ3 resolution (2026-04-26): subagent dispatch creates
  a parent-child link at message-id; question state is per
  session-tree, not per individual session.
- `agent_id` always resolves to the top-of-tree's owner.
