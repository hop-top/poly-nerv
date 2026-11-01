#!/bin/bash
# nerv Shell SDK — source this in your hook handler.
#
# Provides:
#   nerv_read_envelope  Read JSON envelope from stdin; populates NERV_* vars
#   nerv_allow          Output allow decision and exit 0
#   nerv_warn           Output warn decision and exit 1
#   nerv_block          Output block decision and exit 2
#   nerv_rewrite        Output rewrite decision and exit 3
#   nerv_context        Queue model-facing context for the next decision
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

# ── Model context ────────────────────────────────────────────────
# nerv_context <text>: queue text for the model's context window. Call it
# before a decision writer; the decision then carries a "context" field.
# Repeated calls join with newlines; empty text is ignored. Delivery
# depends on the host CLI (see spec/handler-contract.md, "Context").
nerv_context() {
  local text="${1:-}"
  [ -n "$text" ] || return 0
  if [ -n "${NERV_CONTEXT:-}" ]; then
    NERV_CONTEXT="${NERV_CONTEXT}
${text}"
  else
    NERV_CONTEXT="$text"
  fi
}

# ── Decision writers ─────────────────────────────────────────────
# _nerv_emit <action> [message] [rewrite-json]: print one decision line.
_nerv_emit() {
  jq -cn --arg action "$1" --arg msg "${2:-}" \
    --argjson rewrite "${3:-null}" --arg ctx "${NERV_CONTEXT:-}" '
    {action: $action}
    + (if $msg != "" then {message: $msg} else {} end)
    + (if $rewrite != null then {rewrite: $rewrite} else {} end)
    + (if $ctx != "" then {context: $ctx} else {} end)'
}

nerv_allow() {
  _nerv_emit allow
  exit 0
}

nerv_warn() {
  _nerv_emit warn "${1:-}"
  exit 1
}

nerv_block() {
  _nerv_emit block "${1:-}"
  exit 2
}

nerv_rewrite() {
  local payload="${1:?nerv_rewrite requires a JSON payload argument}"
  local compact
  if ! compact=$(printf '%s' "$payload" | jq -ce '.' 2>/dev/null); then
    echo "nerv_rewrite: payload must be valid JSON" >&2
    exit 4
  fi
  _nerv_emit rewrite "" "$compact"
  exit 3
}
