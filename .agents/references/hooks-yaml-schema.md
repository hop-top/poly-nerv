# hooks.yaml schema

Field reference for `.nerv/hooks.yaml`. Ground truth:
[`spec/hooks.schema.json`](../../spec/hooks.schema.json) +
[`internal/config/loader.go`](../../internal/config/loader.go).

## Top-level shape

| Field      | Type   | Req | Notes                                         |
|------------|--------|-----|-----------------------------------------------|
| `version`  | string | yes | Schema version, e.g. `"1.0-draft"`.           |
| `defaults` | object | no  | Applied to every hook unless overridden.      |
| `hooks`    | list   | yes | Ordered list of hook entries.                 |
| `builtin`  | map    | no  | Per-builtin user config (see builtins.yaml).  |
| `bus`      | object | no  | kit/bus adapter, sinks, network, filters.     |

## `defaults`

| Field      | Type    | Default      | Notes                              |
|------------|---------|--------------|------------------------------------|
| `timeout`  | int     | `10`         | Seconds. Minimum 1.                |
| `on_error` | string  | `"continue"` | `continue` or `abort`.             |
| `parallel` | bool    | `false`      | Run same-event hooks in parallel.  |

## `hooks[]` entry

| Field                | Type            | Req | Notes                                                    |
|----------------------|-----------------|-----|----------------------------------------------------------|
| `name`               | string          | yes | Unique within the file. Human-readable.                  |
| `on`                 | string \| list  | yes | Event name(s) — see `references/event-catalog.md`.       |
| `run`                | string          | yes | Path to executable handler. Mode 1 (exec) only.          |
| `match`              | string (regex)  | no  | Tool-name filter; tool-category events only.             |
| `on_exit`            | map             | no  | Exit-code → decision override (see below).               |
| `require_capability` | string          | no  | Skip if CLI adapter lacks this capability/event.         |
| `parallel`           | bool            | no  | Overrides `defaults.parallel`.                           |
| `timeout`            | int             | no  | Overrides `defaults.timeout`.                            |
| `requires`           | list[string]    | no  | Feature flags, e.g. `["full_payload"]` to skip redaction.|
| `disabled`           | bool            | no  | **Forward-looking** — schema/loader land in phase 2.     |

`disabled: true` will short-circuit dispatch for the entry and emit
`{decision: skipped, reason: disabled}` to the event log. The
sidecar `.nerv/disabled.yaml` handles event-level and binding-level
disables.

## `on_exit` (exit-code map)

Default mapping (handlers can override per-entry):

| Exit | Decision  |
|------|-----------|
| `0`  | `allow`   |
| `1`  | `warn`    |
| `2`  | `block`   |
| `3`  | `rewrite` |
| `>=4`| `error` (fail-open: logged, event proceeds) |

## Examples

### Minimal

```yaml
version: "1.0-draft"
hooks:
  - name: block-rm-rf
    on: PreToolUse
    match: "Bash|shell"
    run: ./hooks/block-rm-rf.sh
```

### Multi-event entry

```yaml
version: "1.0-draft"
hooks:
  - name: audit-tool-calls
    on: [PreToolUse, PostToolUse, PostToolUseFailure]
    run: ./hooks/audit.sh
    timeout: 5
```

### With defaults + capability gate + disabled (forward-looking)

```yaml
version: "1.0-draft"
defaults:
  timeout: 15
  on_error: continue
  parallel: true
hooks:
  - name: redact-secrets
    on: UserPromptSubmit
    run: ./hooks/redact.sh
    requires: [full_payload]
  - name: lsp-noise-filter
    on: LspDiagnostics
    run: ./hooks/lsp-filter.sh
    require_capability: LspDiagnostics
    disabled: true
```

## Lockfile — `.nerv/hooks.lock`

SHA-256 lockfile pinning every resolved handler artifact. Written
by `nerv add`. Never hand-edit — `nerv doctor` will flag drift
between `hooks.yaml` entries and the lockfile.
