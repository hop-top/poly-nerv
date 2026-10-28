#!/usr/bin/env bash
# block-rm-rf: blocks rm -rf / and rm -rf /* commands.
# Event: PreToolUse (match: Bash). Contract: spec/handler-contract.md.
set -euo pipefail

ENVELOPE="$(cat)"
CMD="$(printf '%s' "$ENVELOPE" | jq -r '.payload.tool_input.command // empty')"

# Match dangerous rm patterns.
if printf '%s' "$CMD" | grep -qE 'rm[[:space:]]+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)[[:space:]]+/([[:space:]]|$|\*)'; then
  echo '{"action":"block","message":"blocked: rm -rf on root filesystem"}'
  exit 2
fi

echo '{"action":"allow"}'
exit 0
