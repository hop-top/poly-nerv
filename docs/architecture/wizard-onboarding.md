# ADR: wizard onboarding — nerv init

Last updated: 2026-04-26
Author: jadb

## Context

nerv has no interactive onboarding today. New users run
`nerv install <cli>` and read docs to configure hooks. As
built-in hooks land (open-questions, agents-md-linter, …),
users need a one-shot wizard that surfaces enable/disable
choices and writes config accordingly.

Soft dependency for the open-questions hook — that
hook ships toggleable via plain `nerv config set` even
without the wizard. Wizard makes it discoverable; not
blocking.

The builtins registry this wizard reads from is designed
concurrently in the builtins-registry ADR. This
ADR references that registry at the interface level only:
`BuiltinRegistry.List() []BuiltinEntry`. The ADR does NOT
depend on the registry being complete to implement.

Flow diagram: [wizard-flow-v1.mmd](wizard-onboarding/wizard-flow-v1.mmd)
Schema diagram: [builtin-metadata-schema-v1.mmd](wizard-onboarding/builtin-metadata-schema-v1.mmd)

---

## Decision 1 — Entry point: `nerv init`

New top-level subcommand `nerv init`. Registered in
`cmd/nerv/main.go` alongside existing commands.

Rejected alternatives:

- `nerv setup` — implies OS-level setup; conflicts with
  platform idioms
- auto-on-first-invocation — magic behavior is wrong for
  a security-configuring tool; violates fail-open intent

Discoverability: `nerv install <cli>` prints a trailing
hint: `"Run 'nerv init' to configure built-in hooks."`.
`nerv doctor` prints a warning if wizard-state.yaml is
absent and builtins are registered.

---

## Decision 2 — Flow shape: linear per-builtin

One builtin per prompt screen. No category grouping, no
multi-select TUI widget.

Rationale: builtin count at launch is O(5). Category
grouping adds navigation complexity with no UX benefit at
this scale. Revisit if count exceeds 15.

Prompt screen layout (plain terminal, no TUI framework):

```
open-questions — surface unresolved questions from sessions

  Intercepts Stop events; scans response for open question
  patterns; surfaces matches as a post-session summary.

  Events:     Stop (blocking, read-only)
  Risk:       low
  Default:    enabled
  Requires:   any CLI (all adapters)

Enable? [Y/n/skip/quit]
```

`Y` = enable (or accept default).
`n` = disable.
`skip` = defer to next `nerv init` run (not added to seen set).
`quit` = abort; no writes; exit 0.

Charm `lipgloss` (already in `go.mod`) used for risk-level
color coding only: green=low, yellow=medium, red=high. No
full BubbleTea TUI — keeps stdin handling simple and
compatible with CI pipe detection.

---

## Decision 3 — Persistence: user config layer

Enabled builtins are written as `HookEntry` records
appended to `~/.config/nerv/hooks.yaml` (user layer) via
the existing `config.Load` + merge pipeline in
`internal/config/loader.go`.

No new YAML overlay file. No wizard-specific config format.

Wizard run state (idempotence) is tracked separately in
`~/.config/nerv/wizard-state.yaml`. This file is NOT part
of the hook dispatch pipeline — nervd never reads it.

Writing to the user layer preserves layering semantics:
project `.nerv/hooks.yaml` still overrides; system
`/etc/nerv/hooks.yaml` still applies globally.

---

## Decision 4 — Idempotence: seen-set in wizard-state.yaml

`wizard-state.yaml` schema:

```yaml
version: "1"
seen_at: "2026-04-26T12:00:00Z"
detected_cli: claude
seen_builtins:
  - open-questions
```

On re-run, wizard computes:

```
unseen = registry.List() \ state.seen_builtins
```

and prompts only for `unseen`. Builtins in `seen_builtins`
are skipped silently (no re-prompt, even if disabled).

`--reset` flag clears `seen_builtins` to re-prompt all.
Useful after a team policy change.

Skipped builtins (user chose `skip` during prompt) are NOT
added to `seen_builtins` — they appear on next `nerv init`.
This distinguishes "I chose to disable this" from "I never
decided."

---

## Decision 5 — CI / non-interactive mode

Three paths, evaluated in order:

1. `--from <preset.yaml>` — load full `{id: bool}` map;
   apply without prompting; highest priority.
2. `--yes` flag (or `!isatty(os.Stdin)`) — accept all
   defaults for unseen builtins; print one-line per
   builtin; emit JSON summary to stdout for CI parsing.
3. Interactive (default) — TTY present, no flags.

`--yes` and `--from` are mutually exclusive; `PreRunE`
validates and returns an error if both are set.

JSON summary shape (stdout when `--yes` or non-TTY):

```json
{
  "wizard_version": "1",
  "applied": [
    {"id": "open-questions", "enabled": true, "source": "default"}
  ],
  "skipped": [],
  "written_to": "~/.config/nerv/hooks.yaml"
}
```

CI pipeline can `jq .applied` to verify choices.

---

## Decision 6 — Builtin metadata schema

Source file: `spec/builtins.yaml`. Schema per entry:

```yaml
- id: open-questions           # unique; matches hook name
  description: one-line summary for wizard header
  prompt: |                    # multi-line; shown in wizard
    open-questions — surface unresolved questions

    Intercepts Stop events; scans response for open question
    patterns; surfaces matches as a post-session summary.

    Events:     Stop (blocking, read-only)
    Risk:       low
    Default:    enabled
    Requires:   any CLI
  default_enabled: true
  risk_level: low              # low | medium | high
  events:
    - Stop                     # nerv event names from spec/events.yaml
  requires_cli: []             # empty = all CLIs; non-empty = subset
  hook_template: |             # written verbatim to hooks.yaml
    name: open-questions
    on: Stop
    run: ~/.nerv/hooks/open-questions
    timeout: 5
```

`hook_template` is rendered verbatim (no templating
engine). Future: parameterised templates if builtins need
config keys.

`risk_level` semantics:

- `low` — read-only, never blocks, no network calls
- `medium` — may warn or rewrite; local side effects only
- `high` — can block events or make network calls

---

## Decision 7 — Per-CLI adapter awareness

At startup, `nerv init` calls `adapters.Registry()` and
runs `Detect()` on each adapter:

```go
detectedCLIs := []string{}
for name, a := range adapters.Registry() {
    if dr, _ := a.Detect(); dr.Found {
        detectedCLIs = append(detectedCLIs, string(name))
    }
}
```

For each builtin, before prompting:

1. If `requires_cli` non-empty and no intersection with
   `detectedCLIs` → skip; print:
   `"  [skip] open-questions — requires claude or codex (not detected)"`
2. If all of builtin's `events` appear in the
   `unsupported:` list of EVERY detected CLI's
   `capabilities.yaml` → skip with note. (Rare; most
   events are at least synthesised.)

The primary detected CLI (first match in `SupportedCLIs()`
order) is recorded in `wizard-state.yaml` as
`detected_cli`. Used by `nerv doctor` to tailor advice.

---

## Worked example — open-questions hook prompt

Exact text shown in interactive mode (terminal ≥80 cols):

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  open-questions (1/1)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Surface unresolved questions from sessions.

  Intercepts Stop events and scans the response for open
  question patterns. Matches are surfaced as a post-session
  summary so nothing gets lost between turns.

  Events:   Stop (blocking, read-only — never blocks the event)
  Risk:     low
  Default:  enabled
  Requires: any CLI

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Enable? [Y/n/skip/quit]
```

Non-interactive (`--yes` / CI):

```
  ✓ open-questions   enabled (default)
```

---

## Implementation map

### Phase 1 — Foundation (no TUI yet)

- [ ] `spec/builtins.yaml` — schema + open-questions entry
- [ ] `internal/wizard/` package
  - `registry.go` — `BuiltinEntry`, `BuiltinRegistry`
    interface, `LoadRegistry(path string)` from YAML
  - `state.go` — `WizardState` struct, `LoadState()`,
    `SaveState()` at XDG config path; `SeenSet() map[string]bool`
  - `filter.go` — `FilterForCLIs(builtins, detectedCLIs)`
    using `requires_cli`; returns filtered slice
  - `write.go` — `AppendHooks(entries []BuiltinEntry,
    enabled map[string]bool)` reads user hooks.yaml,
    appends new HookEntry records, atomic write
- [ ] `cmd/nerv/cmd_init.go` — cobra command; `--yes`,
  `--from`, `--reset` flags; TTY detection; calls wizard
  package
- [ ] `cmd/nerv/main.go` — add `initCmd()` to `AddCommand`

### Phase 2 — Idempotence + CI polish

- [ ] `internal/wizard/state.go` — skip vs. disable
  distinction; `--reset` clearing
- [ ] JSON summary output for non-TTY (`output.Render` or
  raw `encoding/json` to stdout)
- [ ] `nerv install <cli>` — append discoverability hint
- [ ] `nerv doctor` — add check: wizard-state.yaml absent +
  builtins registered → WARN "run 'nerv init'"

### Phase 3 — Adapter awareness

- [ ] Capabilities loader in `internal/wizard/filter.go`
  — parse `adapters/{cli}/capabilities.yaml`
  `unsupported:` list; cross-reference builtin events
- [ ] `nerv init --verbose` — print per-builtin skip reasons

### Phase 4 — open-questions hook handler

Depends on the open-questions hook (soft dep).

- [ ] `~/.nerv/hooks/open-questions` shell handler
- [ ] E2E cassette: Stop event → open-questions handler →
  warn decision with summary message

---

## Data flow

```
spec/builtins.yaml
  ↓ LoadRegistry()
BuiltinRegistry.List()
  ↓ FilterForCLIs(detectedCLIs)
filtered []BuiltinEntry
  ↓ prompt or --yes defaults
map[id]bool (enabled choices)
  ↓ AppendHooks()
~/.config/nerv/hooks.yaml (user layer)
  ↓ wizard-state.yaml updated
nervd dispatch pipeline (unchanged)
```

---

## Resolved decisions (2026-04-26 founder review)

OQ1–OQ4 resolved during the 2026-04-26 design pass.

1. **Skipped builtins on next run — RESOLVED: re-prompt every run.**
   Aligns with nerv's stateless-per-run philosophy
   (`adapters/internal/install.go:28-49` uses transaction +
   journal idempotence; `cmd/nerv/cmd_doctor.go:47-79` re-runs
   all checks every time). Permanent disable is a separate
   affordance: user sets `enabled: false` in config directly via
   `nerv config set` or future `nerv hook disable`. Skip in wizard
   means "ask me again next time." `--reset` still clears the
   seen-set for team-policy resets.

2. **Wizard installs handler binary — RESOLVED: drop
   `hook_template` for built-ins entirely.** Per builtins-registry ADR §4
   (`builtins-registry.md`), built-ins dispatch in-process via
   `runBuiltin(...)`; `HookEntry.Run` is intentionally empty for
   `Source: "builtin"`. The `hook_template: run: ~/.nerv/hooks/<id>`
   line is dead code for built-ins. `BuiltinEntry.hook_template`
   field is REMOVED from `spec/builtins.yaml` schema (see updated
   schema below + companion change to `spec/builtins.yaml`). For
   future user-installable hooks (none yet), the existing `nerv add
   <id>` path applies — separate concern, not the wizard's job.

3. **Builtins-registry interface contract — RESOLVED: two layers, one sync
   rule.** `internal/hookpkg/builtins/registry.go:All` is the
   dispatch source-of-truth (operational; routing, schema
   versioning, migration). `spec/builtins.yaml` is wizard
   metadata only (descriptive; prompt text, risk level, default,
   CLI requirements). They serve different consumers: daemon vs
   wizard. Synchronization rule: every YAML entry's `id` MUST
   match a `builtins.All.Name`. Wizard startup calls
   `builtins.Validate(yamlRegistry)` and fails loudly if YAML
   references a non-existent built-in. Coupling is read-only YAML
   → code; code never reads YAML.

4. **Primary CLI ordering — RESOLVED: alpha order (no opinion).**
   Use `adapters.SupportedCLIs()` default. Capability ranking
   bakes in opinions about which AI CLI is "best"; nerv is
   CLI-agnostic by design.

### Schema correction (consequence of #2)

`BuiltinEntry.hook_template` field is REMOVED. Updated schema:

```yaml
- id: open-questions
  description: surface unresolved questions from sessions
  prompt: |
    open-questions — surface unresolved questions
    ...
  default_enabled: true
  risk_level: low
  events:
    - Stop
  requires_cli: []
  # hook_template REMOVED — built-ins dispatch in-process via the builtins registry
```

Wizard writes only `enabled: true|false` per built-in to the user
config layer; the daemon reads the registry from `builtins.All`
and routes via `Source: "builtin"`.

### Cross-component impact (newly added)

- **kit/bus dependency surfaced:** Built-ins that emit events
  (e.g. open-questions hook → `inbox_message_*`) require the
  kit/bus adoption to ship phase 1 first. Wizard
  itself has no bus dependency, but documenting this so the
  dependency graph is complete.
