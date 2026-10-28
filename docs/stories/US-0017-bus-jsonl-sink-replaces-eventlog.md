# US-0017: Bus jsonl sink replaces direct eventlog writes

**Persona:** Platform Engineer, OSS Go Developer
**Priority:** P1

## Story

As a platform engineer doing audit queries (or an OSS
developer maintaining nerv), I want all nerv events to flow
through `kit/bus` and the jsonl sink — so there's ONE
event log, not two, and the sink is the bus's
side-effect, not a separate write path.

## Acceptance Criteria

- [ ] Given nervd is running with the jsonl sink configured,
      when ANY event publishes on the bus, then a JSON line
      appears in `$XDG_STATE_HOME/nerv/events.jsonl` within
      100ms with all envelope fields
- [ ] Given the jsonl sink is the ONLY sink configured,
      when nervd emits an event, then there are NO direct
      `eventlog.Append(...)` calls bypassing the bus —
      verified by code grep returning zero matches outside
      the sink itself (and tests)
- [ ] Given `events.jsonl` exists from before bus adoption,
      when bus's jsonl sink starts, then it APPENDS to the
      existing file — does not truncate, does not back up
- [ ] Given the file is rotating (date-based filename),
      when day boundary crosses, then the sink rotates to
      the new date filename WITHOUT dropping the boundary
      event
- [ ] Given audit query `nerv audit --since 2026-04-01
      --event PreToolUse`, when run, then it reads
      `events.jsonl` (and rotated daily files) AND filters
      by event type, returning rows in <5s for a 30-day
      window

## Notes

- Phase 2 of the kit/bus adoption: walks every existing
  `eventlog.Append` call, replaces with `bus.Publish`.
- Eventlog package becomes the SINK implementation, not a
  separate writer.
