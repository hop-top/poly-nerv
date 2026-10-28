#!/usr/bin/env bash
# block-secrets: blocks tool input or output containing common secret
# patterns.
#
# Leverages redaction patterns from the nerv spec to detect leaked
# credentials in tool input (PreToolUse) and tool output (PostToolUse).
set -euo pipefail

ENVELOPE="$(cat)"

# Combine every string in tool_input and tool_response for scanning.
CONTENT="$(echo "$ENVELOPE" | jq -r '
  [(.payload.tool_input // {}), (.payload.tool_response // {})
   | .. | strings] | join("\n")
')"

if [ -z "$CONTENT" ]; then
  echo '{"action":"allow"}'
  exit 0
fi

# Common secret patterns.
PATTERNS=(
  'AKIA[0-9A-Z]{16}'                          # AWS access key
  'sk-[a-zA-Z0-9]{20,}'                       # OpenAI / Stripe secret key
  'ghp_[a-zA-Z0-9]{36}'                       # GitHub PAT
  'glpat-[a-zA-Z0-9\-]{20,}'                  # GitLab PAT
  'xox[bpras]-[a-zA-Z0-9\-]+'                 # Slack token
  '-----BEGIN (RSA |EC |DSA )?PRIVATE KEY-----' # PEM private key
  'AIza[0-9A-Za-z\-_]{35}'                    # Google API key
)

for PAT in "${PATTERNS[@]}"; do
  if printf '%s\n' "$CONTENT" | grep -qE -e "$PAT"; then
    echo '{"action":"block","message":"potential secret detected in tool input or output"}'
    exit 2
  fi
done

echo '{"action":"allow"}'
exit 0
