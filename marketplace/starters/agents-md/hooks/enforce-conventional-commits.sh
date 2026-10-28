#!/usr/bin/env bash
# enforce-conventional-commits: ensures git commit messages follow
# Conventional Commits format (feat|fix|refactor|build|ci|chore|
# docs|style|perf|test).
set -euo pipefail

ENVELOPE="$(cat)"
CMD="$(echo "$ENVELOPE" | jq -r '.payload.tool_input.command // empty')"

# Only inspect git commit commands with -m flag.
if ! echo "$CMD" | grep -q 'git commit.*-m'; then
  echo '{"action":"allow"}'
  exit 0
fi

# Extract the commit message (first -m argument).
# sed -E keeps this portable across GNU and BSD sed.
MSG="$(printf '%s' "$CMD" | sed -nE "s/.*-m[[:space:]]*[\"']?([^\"']*).*/\1/p")"

if [ -z "$MSG" ]; then
  echo '{"action":"allow"}'
  exit 0
fi

# Validate prefix.
CC_PATTERN='^(feat|fix|refactor|build|ci|chore|docs|style|perf|test)(\(.+\))?!?:'
if ! echo "$MSG" | grep -qE "$CC_PATTERN"; then
  echo '{"action":"warn","message":"commit message does not follow Conventional Commits format"}'
  exit 1
fi

echo '{"action":"allow"}'
exit 0
