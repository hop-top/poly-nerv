# spec/builtins.yaml

Last updated: 2026-04-26
Author: $USER
Spec: docs/architecture/wizard-onboarding.md (Decision 6)

Wizard-facing registry of nerv's in-process built-in hooks, for the
onboarding wizard designed in the wizard-onboarding ADR.

> Status: the `nerv init` wizard is not in the CLI yet. Nothing in the
> shipped CLI or daemon reads this file today; it is the data contract
> the wizard will load (one prompt screen per built-in).

The daemon (`nervd`) does NOT read it — built-ins dispatch in-process via
`internal/hookpkg/builtins/registry.go` (builtins-registry ADR Decision 1).

## Field semantics

| Field             | Type        | Notes                                                                                                  |
|-------------------|-------------|--------------------------------------------------------------------------------------------------------|
| `id`              | string      | Unique slug. MUST match a registered `builtins.All.Name`. Pattern `^[a-z][a-z0-9-]*$`.                 |
| `description`     | string      | One-line summary; used in lists and as the prompt header.                                              |
| `prompt`          | string (\|) | Multi-line block shown on the wizard prompt screen. Verbatim, no templating.                           |
| `default_enabled` | boolean     | Wizard pre-selects Enable when true; user can flip with `n`.                                           |
| `risk_level`      | enum        | `low` \| `medium` \| `high`. Drives lipgloss color coding (green/yellow/red). See semantics below.     |
| `events`          | array       | Nerv event names from `spec/events.yaml`. Used for adapter-capability filtering.                       |
| `requires_cli`    | array       | CLI ids (e.g. `claude`, `codex`). Empty = all CLIs; non-empty = wizard prompts only on intersection.   |

### `risk_level` semantics

| Level    | Meaning                                                  |
|----------|----------------------------------------------------------|
| `low`    | Read-only handler; never blocks; no network calls.       |
| `medium` | May warn or rewrite; local side effects only.            |
| `high`   | Can block events or make network calls.                  |

### `requires_cli` semantics

- `[]` (empty)        → built-in is offered regardless of detected CLIs.
- `["claude"]`        → wizard prompts only if `adapters.Detect()` finds claude.
- `["claude","codex"]`→ wizard prompts if EITHER claude OR codex is detected
  (intersection non-empty). Skip line printed when no intersection:
  `[skip] <id> — requires claude or codex (not detected)`.

## Sync rule (do not break)

Every entry's `id` MUST match a registered `builtins.All.Name` in
`internal/hookpkg/builtins/registry.go` (per wizard-onboarding ADR OQ3 resolution).

Wizard startup is to call `builtins.Validate(yamlRegistry)` (not yet
implemented) and abort loudly if a YAML entry references a
non-existent built-in. Coupling is
read-only YAML → code; the daemon never reads YAML.

When adding a built-in:

1. Register a `BuiltinDef` in `internal/hookpkg/builtins/registry.go`.
2. Append a matching entry here with the same `id`.
3. Run `make test` to confirm sync (`builtins.Validate()` once it exists).

When removing a built-in: delete from BOTH places in the same change.

## `hook_template` is intentionally absent

Earlier drafts of this schema included a `hook_template` field rendering
to `~/.config/nerv/hooks.yaml`. Removed during the 2026-04-26 founder review
(wizard-onboarding ADR OQ2 resolution). Built-ins dispatch in-process via
the builtins registry; `HookEntry.Run` is empty for `Source: "builtin"`. The wizard
writes only `enabled: true|false` per built-in to the user config layer.

For future user-installable (out-of-process) hooks, the existing
`nerv add <id>` path applies — separate concern, not the wizard's job.

## Validation

`spec/builtins.schema.json` is the authoritative JSON Schema (draft-07)
for this file. `version` and the schema's `$id` move in lockstep:

| `version` | Schema                                |
|-----------|---------------------------------------|
| `"1"`     | `https://hop.top/nerv/schemas/builtins/1` |

Bump both when entry shape changes; provide a wizard loader migration.

## Related

- ADR: `docs/architecture/wizard-onboarding.md` (Decision 6, OQ resolutions)
- Stories: `docs/stories/US-0009-wizard-init-interactive.md`,
  `docs/stories/US-0011-wizard-cli-aware-filtering.md`
- Events: `spec/events.yaml`
- Registry (code): `internal/hookpkg/builtins/registry.go`
  (ADR: `docs/architecture/builtins-registry.md`)
- Validate hook: `builtins.Validate(yaml-registry)` (planned)
- Loader: `internal/wizard/registry.go` (planned)
