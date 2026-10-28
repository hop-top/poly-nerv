#!/usr/bin/env bash
# nerv Hook Handler Template (Bash)
#
# Reads a JSON envelope from stdin, processes it, and writes
# a JSON response to stdout.
#
# Exit codes (spec/handler-contract.md); the stdout "action" must match:
#   0 — allow   (event proceeds unchanged)
#   1 — warn    (event proceeds; "message" shown to the user)
#   2 — block   (event rejected; "message" shown to the user)
#   3 — rewrite (event proceeds with the "rewrite" payload)
#  >=4 — error  (fail-open: logged, event proceeds)
#
# Event fields live under .payload (spec/events.yaml), e.g.
# .payload.tool_name, .payload.tool_input.command.

set -euo pipefail

# Read the full envelope from stdin.
ENVELOPE="$(cat)"

# Extract fields (requires jq).
EVENT="$(echo "$ENVELOPE" | jq -r '.event // empty')"
TOOL="$(echo "$ENVELOPE" | jq -r '.payload.tool_name // empty')"

# --- Your logic here ---

# Example: log and allow everything. To block instead:
#   echo '{"action":"block","message":"why it was blocked"}'; exit 2
echo "hook: event=$EVENT tool=$TOOL" >&2

# Allow by default.
echo '{"action":"allow"}'
exit 0
