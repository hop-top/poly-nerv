# US-0005: Inbox triage and promote workflow

**Persona:** Solo Developer
**Priority:** P0

## Story

As a solo developer, when an inbox entry exists, I want to
read it in my editor, optionally answer questions inline,
and then run one command to promote checked items into real
tlc tasks (or drop them) — without copy-pasting question
text into `tlc task create`.

## Acceptance Criteria

- [ ] Given an inbox entry exists, when I open it in `$EDITOR`,
      then I see a markdown file with frontmatter (session_id,
      agent_id, trigger, topic_anchor, questions_total,
      questions_unanswered) and per-question sections each
      with: question text, type, suggested-follow-up checkbox
      list (`[ ] Promote to tlc task`, `[ ] Decide inline`,
      `[ ] Defer / drop`)
- [ ] Given I check `[ ] Promote to tlc task` for one or more
      questions and save, when I run `nerv inbox promote
      <inbox-file>`, then for each promoted question a tlc
      task is created in the active scope (`tlc task create`)
      AND the bus emits `inbox_message_promoted` with
      `target == "tlc-task"` and `target_id == <T-NNNN>`
- [ ] Given I check `[ ] Defer / drop` for one or more
      questions, when I run `nerv inbox promote`, then dropped
      questions emit `inbox_message_dropped` with `reason ==
      "user-deleted"`
- [ ] Given all questions in an entry are processed (promoted
      or dropped), when promote completes, then the inbox file
      is removed from disk
- [ ] Given some questions remain unprocessed (no checkbox
      ticked), when promote runs, then those questions stay
      in the file and the file remains on disk
- [ ] Given I edit a question's text inline (answer next to
      the question), when I save, then the bus emits
      `inbox_message_updated` with `fields_changed: ["text"]`
      — no promote required to record the edit

## Notes

- Promote workflow is a CLI command (`nerv inbox promote`), not
  an automatic file-watcher action.
- Bus events let downstream tools (ctxt, observability)
  capture decision history without polling files.
