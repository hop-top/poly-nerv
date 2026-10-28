# Envelope shape

Wire format for every nerv hook event. Ground truth:
[`spec/envelope.schema.json`](../../spec/envelope.schema.json).
Validation is for lint/conformance only — **not** the hot dispatch
path (perf decision).

## Required fields

| Field          | Type    | Notes                                                          |
|----------------|---------|----------------------------------------------------------------|
| `nerv_version` | string  | Protocol semver, e.g. `"0.1.0-alpha.0"`.                       |
| `event`        | string  | Event name from `spec/events.yaml` (see event-catalog.md).     |
| `event_id`     | string  | UUID v4. Unique per emission.                                  |
| `timestamp`    | string  | ISO 8601 with timezone.                                        |
| `cli`          | object  | `{ name, version, binary_path? }`. `name` matches `uxp.CLIName`.|
| `session`      | object  | `{ id, project_key?, cwd? }`.                                  |
| `payload`      | object  | Event-specific shape — see `spec/events.yaml` per event.       |

## Optional fields

| Field               | Type           | Notes                                                                |
|---------------------|----------------|----------------------------------------------------------------------|
| `synthesized`       | bool           | True if the adapter synthesized the event vs. native emission.       |
| `synthesis_source`  | string \| null | Technique: `matcher`, `daemon`, `status_diff`, `binary_wrap`, `mcp_proxy`. |
| `capabilities`      | object \| null | Reference to adapter capability map: `{ cli, cli_version_range }`.   |

## Wire break — `nerv_version`

The field is **`nerv_version`** since `0.1.0-alpha.0`. Older code
referencing `uhp_version` is from before the rename and will not
parse. Regenerate SDKs against the current schema if you see this.

## Size limits

Max envelope: 1 MB soft (configurable via `NERV_MAX_ENVELOPE_SIZE`),
10 MB hard ceiling. Over-limit envelopes are rejected before any
handler runs.

## Synthetic construction

`nerv test` builds envelopes from its flags (`--event`, `--tool`,
`--command`). Synthetic envelopes
flow through the same dispatch path as live events; nothing leaks
back to a CLI. Set `synthesized: true` when constructing by hand so
handlers that branch on it behave correctly.

### Example — PreToolUse (Bash)

```json
{
  "nerv_version": "0.1.0-alpha.0",
  "event": "PreToolUse",
  "event_id": "c7d2f1aa-9c4d-4b2e-9b1a-1a2b3c4d5e6f",
  "timestamp": "2026-05-26T12:00:00Z",
  "cli": { "name": "claude-code", "version": "0.4.2" },
  "session": {
    "id": "abc-123",
    "project_key": "workspace/nerv",
    "cwd": "/home/dev/projects/nerv"
  },
  "payload": {
    "tool_name": "Bash",
    "tool_input": { "command": "rm -rf /" },
    "session_id": "abc-123"
  },
  "synthesized": true,
  "synthesis_source": "matcher"
}
```

### Example — UserPromptSubmit

```json
{
  "nerv_version": "0.1.0-alpha.0",
  "event": "UserPromptSubmit",
  "event_id": "f0e1d2c3-b4a5-4968-8776-6655443322aa",
  "timestamp": "2026-05-26T12:00:01Z",
  "cli": { "name": "gemini-cli", "version": "1.2.0" },
  "session": { "id": "g-7788", "cwd": "/tmp/play" },
  "payload": {
    "prompt": "refactor cmd/nerv/main.go",
    "turn_index": 4,
    "attachments": []
  }
}
```

### Example — SessionStart

```json
{
  "nerv_version": "0.1.0-alpha.0",
  "event": "SessionStart",
  "event_id": "11223344-5566-4778-8899-aabbccddeeff",
  "timestamp": "2026-05-26T12:00:00Z",
  "cli": { "name": "codex-cli", "version": "0.9.1" },
  "session": {
    "id": "cdx-991",
    "project_key": "workspace/nerv",
    "cwd": "/home/dev/src/nerv"
  },
  "payload": {
    "session_id": "cdx-991",
    "resumed": false,
    "project_key": "workspace/nerv",
    "cwd": "/home/dev/src/nerv"
  }
}
```

## Payload duplication note

Some payloads include `session_id` / `project_key` / `cwd` that
duplicate `envelope.session.*`. Canonical session identity lives in
`envelope.session`; the payload-level copies exist for standalone
consumption (codegen templates, cassette stubs).
