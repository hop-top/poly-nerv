# Persona: AI Agent (nerv Extension)

**Primary Role:** Non-human autonomous agent operating inside an
AI CLI (claude, codex, copilot, gemini, opencode). nerv is the
guardrail + observability layer between the agent and the user's
shell; the agent fires hook events and consumes their decisions
to stay safe, productive, and accountable.

---

## Base Goals

- Discover available capabilities at runtime via structured
  metadata.
- Execute multi-step workflows with predictable, machine-readable
  responses at every boundary.
- Maintain session continuity across interruptions without
  losing accumulated context.
- Minimise token/context-window spend per task while preserving
  decision-relevant information.

## Base Pain Points

- Per-CLI hook formats are non-portable; the same guardrail must
  be re-coded for every CLI the user wires.
- Decisions blocked or rewritten by hooks are opaque if the
  decision payload is unstructured.
- Cross-tool federation (one tool reacting to another's events)
  requires polling files; expensive and latency-heavy.

---

## nerv Interaction Patterns

### Hook decision consumption

```pseudocode
event = nerv.fire(envelope: {event_type, session_id, tool_name, payload})
// returns merger.Decision: action ∈ {allow, warn, block, rewrite}
if event.action == "block":
    abort_with_explanation(event.message)
elif event.action == "rewrite":
    apply(event.rewrite); continue
elif event.action == "warn":
    log(event.message); continue
else:  // allow
    continue
```

### Built-in subscription via kit/bus

```pseudocode
subscriber = nerv.bus.subscribe(events: ["inbox_message_added"])
// receives canonical events without polling files
for event in subscriber.stream():
    if event.payload.kind == "open_question":
        record_for_next_turn(event.payload.msg_id)
```

### Subagent dispatch lineage

```pseudocode
// When agent dispatches a subagent (Task tool, etc.), nerv records
// the parent ↔ child link at the originating message-id.
parent_session = nerv.session.parent_of(current_session_id)
if parent_session:
    inherit_question_state(parent_session.open_questions)
```

---

## User Stories

- As an AI agent, I receive structured `merger.Decision` JSON
  from every nerv hook so I never parse human-readable warnings
  to choose my next action.
- As an AI agent, I subscribe to `inbox_message_added` events
  on `kit/bus` so I learn about user-facing notifications
  emitted by other tools (open-questions hook, future tools)
  without tailing files.
- As an AI agent, I follow the parent-session-tree link nerv
  records on subagent dispatch so the parent's open-question
  state is updated when I (the subagent) answer one.
- As an AI agent, I read `CachedConfig.ActiveProfile` in my
  hook runtime context so I know which user / agent identity
  is active without re-querying aps.
- As an AI agent invoking the open-questions detector, my hook
  decisions never block my main flow — the hook is advisory
  (allow + side-effect inbox write).

---

## Success Metrics

- 100% of nerv `merger.Decision` responses parse against the
  documented JSON schema.
- Cross-tool event delivery latency: < 50ms from publish to
  subscriber receive on memory adapter.
- Subagent dispatch → parent-state-update round-trip: < 100ms
  end-to-end including bus event.
- Zero false-positive hook blocks attributable to schema parse
  failures (i.e., hook errors fail-open per nerv contract).
