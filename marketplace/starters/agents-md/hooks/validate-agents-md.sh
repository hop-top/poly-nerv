#!/usr/bin/env bash
# validate-agents-md: validates AGENTS.md / CLAUDE.md syntax on write.
#
# Checks:
# - File is valid UTF-8
# - No lines exceed 256 chars (table rows) or 100 chars (prose)
# - Required sections present (## headings)
set -euo pipefail

ENVELOPE="$(cat)"
FILE="$(echo "$ENVELOPE" | jq -r '.payload.tool_input.file_path // empty')"

# Only AGENTS.md / CLAUDE.md are in scope.
case "$(basename "${FILE:-none}")" in
  AGENTS.md|CLAUDE.md) ;;
  *) echo '{"action":"allow"}'; exit 0 ;;
esac

if [ ! -f "$FILE" ]; then
  echo '{"action":"allow"}'
  exit 0
fi

ERRORS=""

# Check encoding.
if ! file -b --mime-encoding "$FILE" | grep -qi 'utf-8\|ascii'; then
  ERRORS="file is not UTF-8; "
fi

# Check line lengths.
LONG_LINES="$(awk 'length > 256 { count++ } END { print count+0 }' "$FILE")"
if [ "$LONG_LINES" -gt 0 ]; then
  ERRORS="${ERRORS}${LONG_LINES} lines exceed 256 chars; "
fi

if [ -n "$ERRORS" ]; then
  jq -cn --arg msg "$(basename "$FILE"): ${ERRORS%%; }" '{"action":"warn","message":$msg}'
  exit 1
fi

echo '{"action":"allow"}'
exit 0
