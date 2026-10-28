#!/usr/bin/env bash
# Handler: block rm -rf commands.
# Reads envelope from stdin, checks for destructive patterns.
set -euo pipefail

input=$(cat)
cmd=$(echo "$input" | grep -o '"command"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*: *"//;s/"$//')

if echo "$cmd" | grep -qE 'rm\s+-(r|f|rf|fr)\s+/'; then
  echo '{"action":"block","message":"destructive command blocked"}'
  exit 2
fi

echo '{}'
exit 0
