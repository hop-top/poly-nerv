# Cassettes

Deterministic test fixtures for nerv dispatch, adapters, and SDK.

## Format

Each cassette is a JSON file with deterministic key ordering:

```json
{
  "name": "descriptive_snake_case",
  "description": "What this cassette tests",
  "input": { ... },
  "expected": { ... }
}
```

Keys are alphabetically ordered when recorded. This ensures stable
diffs across re-recordings.

## Modes

- **Replay** (default): tests assert actual output matches cassette.
- **Record** (`NERV_RECORD=1`): tests write actual output as new
  cassette content, creating or overwriting the file.

```sh
# Replay (CI, local default)
go test ./internal/...

# Record new golden files
NERV_RECORD=1 go test ./internal/...
```

## Directory layout

```
cassettes/
  dispatch/     # dispatch pipeline cassettes
  install/      # adapter install cassettes
  adapters/     # per-adapter cassettes
    claude/
    gemini/
    codex/
    opencode/
  scenarios/    # golden scenario cassettes
  sdk/          # SDK test cassettes
```

## PR review

Cassette diffs are first-class review artifacts. When a PR changes
behaviour, the cassette diff shows exactly what changed in
input/output pairs. Reviewers should inspect cassette diffs as
carefully as code changes.
