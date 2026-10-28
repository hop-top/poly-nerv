# US-0018: Bus events follow canonical naming rule

**Persona:** OSS Go Developer, Automation Builder
**Priority:** P1

## Story

As an OSS Go developer adding new events (or as an
automation builder publishing events from a custom hook),
I want a documented naming rule that prevents semantic
collisions like `inbox_created` (container) vs
`inbox_message_added` (item) — so subscribers always know
exactly what they're listening for.

## Acceptance Criteria

- [ ] Given an event represents an INSTANCE within a
      container, when its name is chosen, then it follows
      `<container>_<item>_<verb>` — e.g.
      `inbox_message_added`, `tlc_task_claimed`
- [ ] Given an event represents the CONTAINER ITSELF, when
      its name is chosen, then it follows `<entity>_<verb>`
      — e.g. `inbox_created` (the container created), NOT
      `inbox_message_added` (an item within)
- [ ] Given a new event is added to `spec/bus-events.yaml`
      (or equivalent), when CI runs, then a lint check
      validates the name follows ONE of the two patterns
      AND no name conflicts exist
- [ ] Given a subscriber filters by event name, when the
      bus delivers, then ONLY exact name matches are
      delivered — no prefix matching, no glob (those are
      separate filter features if added later)
- [ ] Given documentation in `docs/architecture/kit-bus-adoption.md`,
      when an OSS contributor reads it, then the naming
      rule is on the first page with examples + counter-
      examples

## Notes

- The rule: `<container>_<verb>` for container lifecycle,
  `<container>_<item>_<verb>` for item events
  (kit-bus-adoption ADR §6).
- Reserved names that already follow the rule but are NOT
  used by current built-ins: `inbox_created`,
  `inbox_destroyed` (container lifecycle, future).
