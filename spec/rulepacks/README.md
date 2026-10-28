# Rule packs — declarative guardrail rules

Rule packs are loaded by the `guardrails` built-in at daemon startup
and matched against `PreToolUse` envelopes. See
[`docs/architecture/guardrails-builtin.md`](../../docs/architecture/guardrails-builtin.md)
for the design.

## File layout

```
spec/rulepacks/
  README.md          (this file)
  rulepack.schema.json
  git.yaml
  rm.yaml
  gh.yaml
  curl.yaml
  wget.yaml
  sudo.yaml
```

Each `<name>.yaml` is one pack. Packs are independent; rule ids must
be globally unique across packs.

## Schema

See `rulepack.schema.json`. Top-level shape:

```pseudocode
name: string                # pack name (matches filename minus .yaml)
version: int                # bump on breaking change
description: string
applies_to:
  tool: string              # which envelope.tool_use.name to match
rules:
  - id: string              # globally unique, kebab-case
    description: string
    severity: low | medium | high
    pattern: string         # Go regexp; matched against tool_input.command
    default_mode: allow | block | rewrite | hitl | manager | peer-<N>
    rewrites:               # required if default_mode includes rewrite
      - strategy: ...
        ...
    destructive: bool       # if true, backup before exec
    backup_strategy: ...    # required if destructive
    reason: string          # multi-line; surfaced in the deny/ask UX
```

## Rule modes

| Mode      | Behavior |
|---|---|
| `allow`   | Pass-through (rule disabled) |
| `block`   | Deny with reason |
| `rewrite` | Mutate command, then proceed (still subject to backup if destructive) |
| `hitl`    | Sync prompt user via stderr; daemon blocks until reply |
| `manager` | Async via bus inbox; route to manager agent's profile |
| `peer-N`  | Async via bus inbox; require N peer approvals |

## Rewrite strategies

| Strategy        | Effect | Example |
|---|---|---|
| `strip_flag`    | Remove a flag and its value (if any) | `--no-verify` → gone |
| `replace_token` | Substitute one token for another | `--force` → `--force-with-lease` |
| `prepend_args`  | Prepend args to the command | `--dry-run` injection |

## Backup strategies

See ADR §"Backup contract" for full list. Each declares what state
to capture before letting a destructive command proceed.

## Testing rule packs

Each pack ships with a true-positive + false-positive corpus at
`internal/hookpkg/builtins/guardrails/testdata/<pack>/`:

```
testdata/git/
  match.txt        # one command per line; MUST match
  no-match.txt     # one command per line; MUST NOT match
```

Tests in `guardrails_test.go` walk this corpus per pack.

## Adding a new rule

1. Edit the appropriate pack (or create a new `<name>.yaml`)
2. Add a rule entry with regex + mode + reason
3. Add positive + negative cases to `testdata/<pack>/match.txt` and
   `testdata/<pack>/no-match.txt`
4. Run `go test ./internal/hookpkg/builtins/guardrails/...`
5. If adding `destructive: true`, also add an integration test
   verifying the backup archive is produced
