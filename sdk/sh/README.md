# nerv.sh

Shell SDK for nerv — Universal Hooks Protocol.

## Requirements

- Bash 4+
- `jq`

## Install

`nerv.sh` is a single file; fetch it from the repository and source it
from your handler:

```bash
curl -fsSLO https://raw.githubusercontent.com/hop-top/poly-nerv/main/sdk/sh/nerv.sh
```

## Usage

```bash
#!/bin/bash
source ./nerv.sh

nerv_read_envelope

if [ "$NERV_EVENT" = "PreToolUse" ] && [ "$NERV_TOOL" = "Bash" ]; then
  cmd=$(nerv_payload tool_input.command)
  if echo "$cmd" | grep -q "rm -rf /"; then
    nerv_block "Dangerous command blocked."
  fi
fi

nerv_allow
```

## API

| Function              | Description                                |
|-----------------------|--------------------------------------------|
| `nerv_read_envelope`  | Read stdin; populate `NERV_*` variables    |
| `nerv_payload <path>` | Extract a payload field via jq path (raw string; empty if absent) |
| `nerv_payload_json <path>` | Extract a payload field as JSON (objects, arrays, `null`) |
| `nerv_allow`          | Output allow decision; exit 0              |
| `nerv_warn [msg]`     | Output warn decision; exit 1               |
| `nerv_block [msg]`    | Output block decision; exit 2              |
| `nerv_rewrite <json>` | Output rewrite decision; exit 3            |

## Variables (after `nerv_read_envelope`)

| Variable            | Content                  |
|---------------------|--------------------------|
| `NERV_ENVELOPE`     | Full JSON envelope       |
| `NERV_EVENT`        | Event name               |
| `NERV_EVENT_ID`     | Event UUID               |
| `NERV_CLI`          | CLI name                 |
| `NERV_CLI_VERSION`  | CLI version              |
| `NERV_SESSION_ID`   | Session ID               |
| `NERV_CWD`          | Working directory        |
| `NERV_TOOL`         | Tool name (if present)   |
