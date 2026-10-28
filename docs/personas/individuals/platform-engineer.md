# Persona: Platform Engineer (nerv Extension)

**Primary Role:** Owns the AI CLI toolchain for a team or org.
Standardises hooks across heterogeneous CLIs, ships built-ins
as defaults, runs observability tooling that subscribes to nerv
events for audit + capacity tracking.

---

## Base Goals

- Provide a uniform layer over heterogeneous developer tools
  so engineers don't reinvent integrations per project.
- Make AI-assisted work auditable and shareable across the
  team without leaking secrets.
- Reduce duplicated investigation across engineers by making
  past decisions discoverable.

## Base Pain Points

- Every engineer's machine has a different hook setup;
  inconsistent enforcement of guardrails across the team.
- No central audit of which hooks blocked which commands;
  postmortem reconstruction is forensic, not queryable.
- New built-ins arrive but no rollout plan: enable for whom?
  with what defaults? how to gate?
- Observability tooling has no clean way to subscribe to nerv
  decisions; current options are tail JSONL files.

---

## nerv Interaction Patterns

### Org-level rollout via wizard preset

```pseudocode
$ cat team-defaults.yaml
hooks.builtin.open-questions.enabled: true
hooks.builtin.<future-builtin>.enabled: false
hooks.builtin.open-questions.llm.budget_tokens: 1000

$ nerv init --from team-defaults.yaml --yes
// applies preset silently; prints JSON summary for CI
```

### Bus subscription for observability

```pseudocode
// observability tool subscribes to nerv's kit/bus
subscriber = nerv.bus.subscribe(events: [
    "inbox_message_added",
    "inbox_message_promoted",
    "PreToolUse",
    "Stop"
], adapter: "network")

// receives canonical events; pushes to APM / SIEM / dashboards
```

### Audit query

```pseudocode
// kit/bus jsonl sink writes to $XDG_STATE_HOME/nerv/events.jsonl
$ nerv audit --since 2026-04-01 --event PreToolUse --action block
// returns blocked tool calls with envelope context
```

### Built-in registry inspection

```pseudocode
$ nerv hook list --format json
// lists user hooks AND built-ins side-by-side
// each with: name, source (user|builtin), events, status (enabled|disabled)
```

---

## User Stories

- As a platform engineer, I roll out the open-questions
  built-in to my team via `nerv init --from <preset.yaml>
  --yes` so all engineers get consistent configuration in one
  command.
- As a platform engineer, I subscribe my observability tool
  to `nerv.bus` events so audit, alerting, and capacity
  tracking work without per-engineer instrumentation.
- As a platform engineer, I run `nerv hook list` to inventory
  what's running on a given engineer's machine without SSH-ing
  in to inspect config files.
- As a platform engineer, I audit which hook decisions blocked
  which commands across my team's recent work via the
  bus-driven `events.jsonl` sink, queryable by event type +
  action.
- As a platform engineer, I gate a new built-in's rollout via
  config layering: enable in `~/.config/nerv/hooks.yaml`
  globally, override to disabled per-project for sensitive
  repos.
- As a platform engineer, I subscribe to
  `inbox_message_promoted` events to feed completed
  open-questions promotions into my team's task tracker, so
  decision history is captured automatically.

---

## Success Metrics

- Mean time to roll out a built-in to a 10-engineer team:
  < 5 minutes (one preset.yaml + ssh + `nerv init --from --yes`).
- Audit query latency for "all blocked PreToolUse events in
  last week": < 5s against `events.jsonl` sink.
- Zero per-engineer hook drift: `nerv hook list` output is
  byte-identical across machines after preset rollout.
- Bus subscriber uptime: ≥ 99.9% — no event loss on
  observability tooling under normal operation.
