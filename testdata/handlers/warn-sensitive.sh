#!/usr/bin/env bash
# Handler: warn on sensitive file access.
set -euo pipefail

input=$(cat)
path=$(echo "$input" | grep -o '"file_path"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*: *"//;s/"$//')

if echo "$path" | grep -qE '\.(env|pem|key)$'; then
  echo '{"action":"warn","message":"accessing sensitive file: '"$path"'"}'
  exit 0
fi

echo '{}'
exit 0
