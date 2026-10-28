#!/usr/bin/env bash
# slow-query-warn: warns if a shell command took longer than 30s.
#
# Event: PostToolUse (match: Bash). Reads payload.duration_ms and
# warns when it exceeds the threshold. Contract: spec/handler-contract.md.
set -euo pipefail

THRESHOLD="${NERV_SLOW_THRESHOLD:-30}"

ENVELOPE="$(cat)"
DURATION_MS="$(printf '%s' "$ENVELOPE" | jq -r '.payload.duration_ms // 0 | floor')"
CMD="$(printf '%s' "$ENVELOPE" | jq -r '.payload.tool_input.command // "unknown"')"

if [ "$DURATION_MS" -gt $(( THRESHOLD * 1000 )) ]; then
  jq -cn --arg msg "slow command ($(( DURATION_MS / 1000 ))s): $CMD" \
    '{"action":"warn","message":$msg}'
  exit 1
fi

echo '{"action":"allow"}'
exit 0
