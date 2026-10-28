# Nerv Redaction Specification

> Best-effort guarantee: secrets should not reach handlers unless
> explicitly opted in. On redaction timeout, envelope is rejected
> (fail-safe) rather than delivered un-redacted.
> Patterns defined in `redaction.yaml`; this doc covers semantics.

## Defaults

- Redaction is **ON** for all hooks.
- Handlers opt into full payload via `requires: ["full_payload"]`
  in `hooks.yaml`.
- When `full_payload` requested, redaction skipped entirely for
  that handler. Handler assumes responsibility for secret hygiene.

## Execution Model

### Pipeline position

```
CLI event → Adapter → Envelope build → **Redaction** → Dispatch → Handler
```

Redaction runs BEFORE the envelope reaches handler stdin. Handlers
never see raw secrets unless they declared `full_payload`.

### Ordering

Patterns applied in definition order from `redaction.yaml`. First
match wins per character span — overlapping matches resolved by
earliest pattern in file.

### Performance

- Budget: redaction MUST complete within **100ms** per envelope.
- On timeout: fail-open — deliver the original (unredacted) envelope
  and log a warning. Never block dispatch.
- Patterns compiled to a regex set at startup; no per-envelope
  compilation.

## Write/Edit Tool Content

For `PreToolUse` and `PostToolUse` events where the tool is
`Write` or `Edit`:

- `payload.content` replaced with a content summary:

  ```json
  {"sha256": "<hex-digest>", "bytes": 4096}
  ```

- Original content never serialized to handler stdin.
- Hash allows handlers to detect content changes across events
  without seeing the content.

## Pattern Categories

### Credentials & tokens

| Pattern             | Matches                          |
|---------------------|----------------------------------|
| `bearer_token`      | `Bearer <token>` strings         |
| `api_key_pattern`   | sk_, pk_, api_, key_, token_ etc  |
| `jwt_token`         | 3-segment base64url JWT          |
| `aws_access_key`    | `AKIA` + 16 alphanum             |
| `aws_secret_key`    | 40-char key after config label   |
| `github_token`      | ghp_, gho_, ghs_, ghr_, pat_     |
| `slack_token`       | xoxb-, xoxp-, xoxe-, xoxa-       |
| `base64_secret`     | base64 value after secret labels |

### Structured secrets

| Pattern              | Matches                         |
|----------------------|---------------------------------|
| `private_key_block`  | PEM private key blocks          |
| `connection_string`  | DB/service URIs with creds      |
| `dotenv_content`     | KEY=value lines from .env       |

### Environment

| Pattern              | Matches                         |
|----------------------|---------------------------------|
| `env_var_expansion`  | `$VAR` / `${VAR}` expansions    |

### Tool-specific

| Pattern              | Matches                         |
|----------------------|---------------------------------|
| `write_tool_content` | Write/Edit payload.content      |

## Replacement Format

All replacements use the `${REDACTED:<label>}` format:

```
Bearer ${REDACTED:token}
${REDACTED:api_key}
${REDACTED:jwt}
```

Labels identify which pattern matched — useful for debugging and
audit logging.

## Testing Requirements

Each pattern MUST have:

1. **Positive tests** — at least 3 known-secret inputs that trigger
   redaction.
2. **Negative tests** — at least 3 known-good inputs that MUST NOT
   trigger false positives.
3. Tests live in the implementation's redaction test suite, using
   the repository's standard test layout and tooling.

Example false-positive guards:

- `api_key_pattern` must not match `application_context` (too short)
- `jwt_token` must not match arbitrary dotted identifiers like
  `com.example.service`
- `connection_string` must not match `postgresql://localhost` (no creds)
- `env_var_expansion` must not match `$0` in regex replacement strings

## Configuration

### hooks.yaml integration

```yaml
hooks:
  - event: PreToolUse
    matcher: ".*"
    handler: "./my-handler"
    requires: []              # default: redaction ON

  - event: PreToolUse
    matcher: "Bash"
    handler: "./audit-handler"
    requires: ["full_payload"] # redaction OFF for this handler
```

### Custom patterns

Users may extend `redaction.yaml` with project-local patterns:

```yaml
# .nerv/redaction.yaml (project-level)
extends: default
patterns:
  - name: internal_api_token
    description: "Internal service tokens"
    regex: 'svc_[a-f0-9]{32}'
    replacement: '${REDACTED:internal_token}'
    scope: payload
```

Project patterns appended after default patterns. Same
first-match-wins semantics.

## Error Handling

| Condition                  | Behavior                        |
|----------------------------|---------------------------------|
| Pattern regex invalid      | Skip pattern; log error at init |
| Redaction timeout (>100ms) | Fail-open; deliver unredacted   |
| Replacement produces empty | Preserve original span          |
| `full_payload` + redaction | `full_payload` wins; skip all   |

## Audit

Every redaction event logged to event store:

```json
{
  "redaction_applied": true,
  "patterns_matched": ["bearer_token", "api_key_pattern"],
  "spans_redacted": 2,
  "elapsed_ms": 3
}
```

Logging never includes the redacted content itself.
