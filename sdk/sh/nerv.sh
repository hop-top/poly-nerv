#!/bin/bash
# nerv Shell SDK — source this in your hook handler.
#
# Provides:
#   nerv_read_envelope  Read JSON envelope from stdin; populates NERV_* vars
#   nerv_allow          Output allow decision and exit 0
#   nerv_warn           Output warn decision and exit 1
#   nerv_block          Output block decision and exit 2
#   nerv_rewrite        Output rewrite decision and exit 3
#
# Requires: jq
#
# Usage:
#   #!/bin/bash
#   source nerv.sh
#   nerv_read_envelope
#   if [ "$NERV_EVENT" = "PreToolUse" ] && [ "$NERV_TOOL" = "Bash" ]; then
#     nerv_block "Bash tool blocked by policy"
#   fi
#   nerv_allow

# Do not set shell options here: this file is intended to be sourced.
# Handlers may enable `set -euo pipefail` themselves if desired.

# ── Read envelope from stdin ─────────────────────────────────────
# Call without piping: nerv_read_envelope (reads stdin directly)
# Or pass JSON as argument: nerv_read_envelope "$json"
nerv_read_envelope() {
  if [ $# -gt 0 ]; then
    NERV_ENVELOPE="$1"
  else
    NERV_ENVELOPE=$(cat)
  fi
  NERV_EVENT=$(printf '%s' "$NERV_ENVELOPE" | jq -r '.event')
  NERV_EVENT_ID=$(printf '%s' "$NERV_ENVELOPE" | jq -r '.event_id')
  NERV_CLI=$(printf '%s' "$NERV_ENVELOPE" | jq -r '.cli.name')
  NERV_CLI_VERSION=$(printf '%s' "$NERV_ENVELOPE" | jq -r '.cli.version')
  NERV_SESSION_ID=$(printf '%s' "$NERV_ENVELOPE" | jq -r '.session.id')
  NERV_CWD=$(printf '%s' "$NERV_ENVELOPE" | jq -r '.session.cwd // empty')
  NERV_TOOL=$(printf '%s' "$NERV_ENVELOPE" | jq -r '.payload.tool_name // empty')
  export NERV_ENVELOPE NERV_EVENT NERV_EVENT_ID NERV_CLI NERV_CLI_VERSION
  export NERV_SESSION_ID NERV_CWD NERV_TOOL
}

# ── Payload helpers ──────────────────────────────────────────────
nerv_payload() {
  printf '%s' "$NERV_ENVELOPE" | jq -r ".payload.$1 // empty"
}

nerv_payload_json() {
  printf '%s' "$NERV_ENVELOPE" | jq ".payload.$1"
}

# ── Decision writers ─────────────────────────────────────────────
nerv_allow() {
  echo '{"action":"allow"}'
  exit 0
}

nerv_warn() {
  local msg="${1:-}"
  if [ -n "$msg" ]; then
    jq -cn --arg msg "$msg" '{"action":"warn","message":$msg}'
  else
    echo '{"action":"warn"}'
  fi
  exit 1
}

nerv_block() {
  local msg="${1:-}"
  if [ -n "$msg" ]; then
    jq -cn --arg msg "$msg" '{"action":"block","message":$msg}'
  else
    echo '{"action":"block"}'
  fi
  exit 2
}

nerv_rewrite() {
  local payload="${1:?nerv_rewrite requires a JSON payload argument}"
  local compact
  if ! compact=$(printf '%s' "$payload" | jq -ce '.' 2>/dev/null); then
    echo "nerv_rewrite: payload must be valid JSON" >&2
    exit 4
  fi
  printf '{"action":"rewrite","rewrite":%s}\n' "$compact"
  exit 3
}
