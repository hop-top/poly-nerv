# Migration: state directory layout

The hook eventlog moved out of the flat `$XDG_STATE_HOME/nerv/` root
into the segregated state layout introduced with the crtx producer
(see [`../architecture/crtx-producer.md`](../architecture/crtx-producer.md)):

```
$XDG_STATE_HOME/nerv/
  events/    internal surface: hook eventlog (events-YYYY-MM-DD.jsonl),
             bus sink/adapter artifacts (bus.jsonl, bus.sqlite)
  sessions/  interchange surface: crtx v0.1 envelopes, one
             <envelope-id>.jsonl per session — the only subtree crtx
             consumers scan
```

## Who is affected

Deployments that ran `nerv` before this change and want `nerv tail`,
`nerv replay`, or `nerv doctor` to keep seeing pre-migration events.
New deployments need no action — directories are created on first
write.

## Steps

Move the date-rotated eventlog files into the new location:

```sh
state="${XDG_STATE_HOME:-$HOME/.local/state}"
mkdir -p "$state/nerv/events"
mv "$state/nerv"/events-*.jsonl "$state/nerv/events/" 2>/dev/null || true
```

Notes:

- `nerv tail` / `nerv replay` / `nerv doctor` read only the new
  location; unmoved files are simply invisible to them (nothing is
  deleted).
- Other state directly under `$XDG_STATE_HOME/nerv/` (daemon pidfile,
  guardrails backups) is unaffected by this migration.
- Do not move anything into `$XDG_STATE_HOME/nerv/sessions/` — that
  subtree is reserved for crtx envelope JSONL; foreign files there
  are skipped by consumers but pollute the interchange surface.
