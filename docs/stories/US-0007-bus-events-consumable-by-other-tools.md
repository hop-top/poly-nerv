# US-0007: Inbox events consumable by other tools via bus

**Persona:** Platform Engineer, AI Agent, Automation Builder
**Priority:** P1

## Story

As a platform engineer (or agent / automation builder),
I want to subscribe to `inbox_message_*` events on nerv's
bus and react in real time — without tailing files or
polling — so my observability tool, ticketing pipeline, or
companion agent can integrate cleanly.

## Acceptance Criteria

- [ ] Given the open-questions hook publishes
      `inbox_message_added`, when an in-process subscriber
      registered via `kit/bus` memory adapter is listening,
      then it receives the event within 50ms
- [ ] Given a subscriber is configured to write events to
      `$XDG_STATE_HOME/nerv/events.jsonl` (jsonl sink), when an
      event publishes, then a JSON line appears in the file
      within 100ms with the canonical schema
- [ ] Given downstream tools subscribe via the network
      adapter (phase 3), when an event publishes, then
      remote subscribers receive it (latency adapter-dependent)
- [ ] Given event names follow the documented semantic rule
      (`<container>_<item>_<verb>`), when I subscribe to
      `inbox_message_added`, then I do NOT receive
      `inbox_created` (container lifecycle) or any other
      unrelated event
- [ ] Given the bus is unavailable (kit/bus init failed),
      when the hook runs, then it logs a warning and writes
      the inbox FILE anyway — bus is a side channel, not the
      primary delivery

## Notes

- Depends on kit/bus adoption shipping phase 1.
- 4 canonical events: `inbox_message_added`,
  `inbox_message_updated`, `inbox_message_promoted`,
  `inbox_message_dropped`. NOT `inbox_created`.
- Event-naming rule: `<container>_<verb>` for container
  lifecycle, `<container>_<item>_<verb>` for item events
  (kit-bus-adoption ADR §6).
