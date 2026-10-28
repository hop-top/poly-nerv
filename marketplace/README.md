# nerv Hook Marketplace

Create and publish hooks for nerv — the Universal Hooks Protocol.

## Hook Package Structure

```
my-hook/
  hook.yaml      # metadata: name, version, events, requirements
  handler.sh     # handler script (bash, python, etc.)
  README.md      # usage + configuration docs
  LICENSE         # license file
```

## hook.yaml Format

```yaml
name: my-hook
version: "1.0.0"
description: Short description of what the hook does
author: your-name
license: MIT
events:            # canonical names from spec/events.yaml
  - PreToolUse
  - PostToolUse
requires:
  capabilities:
    - fs_read
  nerv_version: ">=1.0.0"
```

## Creating a Hook

1. Copy `template/` as a starting point
2. Edit `hook.yaml` with your metadata
3. Implement your handler (see `handler.sh` or `handler.go`). It reads
   the envelope on stdin (event fields under `.payload`), writes a
   decision with `action` and, for warn/block, `message`, and exits
   0 allow / 1 warn / 2 block / 3 rewrite
   ([handler contract](../spec/handler-contract.md))
4. Register it (`nerv add my-hook --source ./my-hook/handler.sh`), then
   test locally: `nerv test my-hook --event PreToolUse --tool Bash --command "echo hi"`
5. Publish: push to a git repo or host the handler at a URL

## Installing Hooks

```sh
# From a URL
nerv add my-hook --source https://example.com/hooks/my-hook

# From a local path
nerv add my-hook --source ./path/to/handler.sh
```

## Examples

See `examples/` for complete, working hooks:

- **block-rm-rf** — blocks destructive `rm -rf /` commands
- **audit-log** — logs all tool calls to a file
- **slow-query-warn** — warns on shell commands exceeding 30s

## Curated picker

Running `/nerv-add` (no arguments) walks `examples/` and `starters/`,
parses each entry's `hook.yaml` / `hooks.yaml` + README, and presents
a multi-select grouped by category. Selected picks install via
`nerv add <name> --source marketplace/<category>s/<name>/`. Each
entry self-describes — there is no central index to keep in sync.
