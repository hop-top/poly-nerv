#!/usr/bin/env bats
# Tests for nerv.sh — the nerv Shell SDK.

SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"

setup() {
  # Source nerv.sh in a subshell-friendly way for each test.
  export NERV_ENVELOPE='{"nerv_version":"1.0.0-draft","event":"PreToolUse","event_id":"abc-123","timestamp":"2026-04-11T12:00:00Z","cli":{"name":"claude-code","version":"1.0.0"},"session":{"id":"sess-1","cwd":"/tmp"},"payload":{"tool_name":"Bash","tool_input":{"command":"ls"}},"synthesized":false}'
}

@test "nerv_allow outputs allow JSON" {
  run bash -c "source '$SCRIPT_DIR/nerv.sh' && nerv_allow"
  [ "$status" -eq 0 ]
  [ "$output" = '{"action":"allow"}' ]
}

@test "nerv_warn outputs warn JSON with message" {
  run bash -c "source '$SCRIPT_DIR/nerv.sh' && nerv_warn 'be careful'"
  [ "$status" -eq 1 ]
  [ "$output" = '{"action":"warn","message":"be careful"}' ]
}

@test "nerv_warn outputs warn JSON without message" {
  run bash -c "source '$SCRIPT_DIR/nerv.sh' && nerv_warn"
  [ "$status" -eq 1 ]
  [ "$output" = '{"action":"warn"}' ]
}

@test "nerv_block outputs block JSON with message" {
  run bash -c "source '$SCRIPT_DIR/nerv.sh' && nerv_block 'not allowed'"
  [ "$status" -eq 2 ]
  [ "$output" = '{"action":"block","message":"not allowed"}' ]
}

@test "nerv_block outputs block JSON without message" {
  run bash -c "source '$SCRIPT_DIR/nerv.sh' && nerv_block"
  [ "$status" -eq 2 ]
  [ "$output" = '{"action":"block"}' ]
}

@test "nerv_rewrite outputs rewrite JSON" {
  run bash -c "source '$SCRIPT_DIR/nerv.sh' && nerv_rewrite '{\"tool_input\":{\"command\":\"ls -la\"}}'"
  [ "$status" -eq 3 ]
  echo "$output" | jq -e '.action == "rewrite"'
  echo "$output" | jq -e '.rewrite.tool_input.command == "ls -la"'
}

@test "nerv_read_envelope parses event and tool" {
  run bash -c "
    source '$SCRIPT_DIR/nerv.sh'
    nerv_read_envelope '$NERV_ENVELOPE'
    echo \"\$NERV_EVENT|\$NERV_TOOL|\$NERV_CLI\"
    nerv_allow
  "
  [ "$status" -eq 0 ]
  [[ "${lines[0]}" == "PreToolUse|Bash|claude-code" ]]
}

@test "nerv_payload extracts field" {
  run bash -c "
    source '$SCRIPT_DIR/nerv.sh'
    nerv_read_envelope '$NERV_ENVELOPE'
    nerv_payload tool_name
  "
  [ "$output" = "Bash" ]
}
