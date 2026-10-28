#!/usr/bin/env bash
# audit-log: appends every tool call to an audit log file.
# Event: PreToolUse. Contract: spec/handler-contract.md.
set -euo pipefail

LOGFILE="${NERV_AUDIT_LOG:-.nerv/audit.log}"

ENVELOPE="$(cat)"
TIMESTAMP="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
EVENT="$(printf '%s' "$ENVELOPE" | jq -r '.event // "unknown"')"
TOOL="$(printf '%s' "$ENVELOPE" | jq -r '.payload.tool_name // "n/a"')"

# Ensure log directory exists.
mkdir -p "$(dirname "$LOGFILE")"

# Append structured log line.
printf '%s event=%s tool=%s\n' "$TIMESTAMP" "$EVENT" "$TOOL" >> "$LOGFILE"

# Always allow — audit is passive.
echo '{"action":"allow"}'
exit 0
