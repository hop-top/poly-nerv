#!/usr/bin/env bash
# Handler: rewrite rm to trash.
set -euo pipefail

input=$(cat)
cmd=$(echo "$input" | grep -o '"command"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*: *"//;s/"$//')

if echo "$cmd" | grep -qE '^rm\s'; then
  rewritten=$(echo "$cmd" | sed 's/^rm /trash /')
  echo '{"action":"rewrite","rewrite":{"command":"'"$rewritten"'"}}'
  exit 3
fi

echo '{}'
exit 0
