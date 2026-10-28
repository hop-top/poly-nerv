#!/usr/bin/env bash
# warn-large-writes: warns when a file write exceeds 100KB.
set -euo pipefail

THRESHOLD="${NERV_LARGE_WRITE_KB:-100}"

ENVELOPE="$(cat)"
FILE="$(echo "$ENVELOPE" | jq -r '.payload.tool_input.file_path // empty')"

if [ -z "$FILE" ] || [ ! -f "$FILE" ]; then
  echo '{"action":"allow"}'
  exit 0
fi

SIZE_KB="$(( $(wc -c < "$FILE") / 1024 ))"

if [ "$SIZE_KB" -gt "$THRESHOLD" ]; then
  jq -cn --arg msg "large file write: ${FILE} is ${SIZE_KB}KB (threshold: ${THRESHOLD}KB)" \
    '{"action":"warn","message":$msg}'
  exit 1
fi

echo '{"action":"allow"}'
exit 0
