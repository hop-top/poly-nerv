# US-0016: Bus publish/subscribe round-trip

**Persona:** AI Agent, Platform Engineer, Automation Builder
**Priority:** P0

## Story

As an AI agent or platform-engineer's observability tool,
I want to subscribe to nerv's bus and receive events
published from anywhere in nervd (built-ins, dispatch,
external producers) without polling files, so cross-tool
integration is event-driven not poll-driven.

## Acceptance Criteria

- [ ] Given nervd is running with `kit/bus` memory adapter,
      when an in-process subscriber registers via
      `bus.Subscribe(eventType)`, then it receives every
      future publish of that event type within 10ms
- [ ] Given a subscriber registered for `inbox_message_added`,
      when an unrelated event (e.g. `Stop`) publishes, then
      the subscriber is NOT called — exact-match routing
- [ ] Given a slow subscriber (handler takes 500ms+), when
      events publish in rapid succession, then the bus does
      NOT block other subscribers AND does NOT drop events
      under default config (memory adapter buffers)
- [ ] Given a subscriber returns an error from its handler,
      when the bus invokes it, then the bus logs the error
      and continues delivering to OTHER subscribers — one
      bad subscriber doesn't break the bus
- [ ] Given the daemon shuts down (SIGTERM), when in-flight
      events are queued, then the bus drains them with a
      configurable timeout (default 5s) before exiting

## Notes

- Memory adapter is default; jsonl adapter writes to
  `$XDG_STATE_HOME/nerv/events.jsonl`; sqlite + network
  adapters phase 3.
- Adapter selection via `kit/config`.
