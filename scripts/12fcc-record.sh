#!/usr/bin/env bash
# Record 12fcc conformance cassettes from real nerv runs.
#
# For every scenario under e2e/conformance/scenarios/nerv/, this
# script executes the scenario's steps against a freshly built binary
# in an isolated temp project and captures genuine stdout, stderr,
# exit code, and duration into the cassette layout the grading
# service consumes:
#
#   e2e/conformance/cassettes/<scenario-id>/
#     manifest.yaml
#     story.yaml
#     steps/<step-id>/{result.json,stdout.txt,stderr.txt}
#
# Captures are never edited after the fact. Re-running the script
# re-records everything from scratch. Recorder automation (xrr) is
# not wired yet; the manifest marks these captures as hand-assembled
# in the xrr cassette layout via recorder_version 0.0.0-manual.
#
# Usage: scripts/12fcc-record.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CONF="$REPO_ROOT/e2e/conformance"
CASSETTES="$CONF/cassettes"
STORIES="$CONF/stories"
BIN="$REPO_ROOT/bin/nerv"

now_ms() { python3 -c 'import time; print(int(time.time()*1000))'; }
utc_now() { date -u +%Y-%m-%dT%H:%M:%SZ; }
sha256_of() { shasum -a 256 "$1" | cut -d' ' -f1; }

echo "==> building $BIN"
(cd "$REPO_ROOT" && go build -buildvcs=false -o "$BIN" ./cmd/nerv)

BINARY_VERSION="$(cd "$REPO_ROOT" && git rev-parse --short HEAD 2>/dev/null || echo dev)"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/nerv-12fcc-record.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

# --- per-scenario state ---------------------------------------------------
SCENARIO_ID=""
PROJECT=""
STEP_IDS=""

begin_scenario() {
  SCENARIO_ID="$1"
  STEP_IDS=""
  PROJECT="$WORK/$SCENARIO_ID"
  mkdir -p "$PROJECT"
  rm -rf "$CASSETTES/$SCENARIO_ID"
  mkdir -p "$CASSETTES/$SCENARIO_ID/steps"
  echo "==> recording $SCENARIO_ID"
}

# run_step <step-id> <arg>... — execute the binary with the given args
# inside the scenario's project dir and capture the outcome verbatim.
run_step() {
  local sid="$1"; shift
  local dir="$CASSETTES/$SCENARIO_ID/steps/$sid"
  mkdir -p "$dir"
  local t0 t1 code
  t0="$(now_ms)"
  set +e
  (cd "$PROJECT" && "$BIN" "$@" >"$dir/stdout.txt" 2>"$dir/stderr.txt")
  code=$?
  set -e
  t1="$(now_ms)"
  printf '{"exit_code": %d, "duration_ms": %d}\n' "$code" "$((t1 - t0))" >"$dir/result.json"
  STEP_IDS="$STEP_IDS $sid"
  echo "    $sid: exit=$code"
}

# setup_run <arg>... — run the binary in the project dir as scenario
# setup (preconditions), without capturing a step.
setup_run() {
  (cd "$PROJECT" && "$BIN" "$@" >/dev/null 2>&1) || true
}

finish_scenario() {
  local story_file="$STORIES/$SCENARIO_ID.yaml"
  local out="$CASSETTES/$SCENARIO_ID"
  cp "$story_file" "$out/story.yaml"

  local story_id story_hash declared
  story_id="$(sed -n 's/^story_id: //p' "$story_file")"
  story_hash="$(sha256_of "$out/story.yaml")"
  declared="$(sed -n 's/.*content_hash: "sha256:\([0-9a-f]*\)".*/\1/p' \
    "$CONF/scenarios/nerv/$SCENARIO_ID/1.0.0/scenario.yaml")"
  if [ "$story_hash" != "$declared" ]; then
    echo "ERROR: story hash drift for $SCENARIO_ID" >&2
    echo "  story file: sha256:$story_hash" >&2
    echo "  scenario.yaml declares: sha256:$declared" >&2
    echo "  update the scenario's story_ref.content_hash" >&2
    exit 1
  fi

  {
    echo 'schema_version: "1"'
    echo 'binary: nerv'
    echo "binary_version: $BINARY_VERSION"
    echo 'recorder: xrr'
    echo 'recorder_version: 0.0.0-manual'
    echo "recorded_at: $(utc_now)"
    echo "scenario_id: nerv/$SCENARIO_ID"
    echo 'scenario_version: 1.0.0'
    echo 'story_ref:'
    echo "  story_id: $story_id"
    echo '  story_path: story.yaml'
    echo "  content_hash: sha256:$story_hash"
    echo 'steps:'
    for sid in $STEP_IDS; do
      echo "  - id: $sid"
      echo "    cassette_dir: steps/$sid/cassette"
      echo "    captures: steps/$sid"
    done
  } >"$out/manifest.yaml"
}

with_handler() {
  printf '#!/bin/sh\necho ok\n' >"$PROJECT/handler.sh"
  chmod +x "$PROJECT/handler.sh"
}

# --- scenarios ------------------------------------------------------------
# Step ids and invokes mirror e2e/conformance/scenarios/nerv/<id>/1.0.0/
# scenario.yaml. Keep both in sync.

begin_scenario status-json
run_step status_json status --format json
finish_scenario

begin_scenario doctor-json
run_step doctor_json doctor --format json
finish_scenario

begin_scenario add-dry-run-preview
with_handler
run_step add_dry add demo-hook --source ./handler.sh --dry-run --format json
run_step status_after_dry status --format json
finish_scenario

begin_scenario add-idempotent-reapply
with_handler
run_step add_first add demo-hook --source ./handler.sh --format json
run_step add_again add demo-hook --source ./handler.sh --format json
run_step status_converged status --format json
run_step remove_absent remove ghost-hook --confirm yes --format json
finish_scenario

begin_scenario error-envelope-correction
run_step replay_missing replay evt-does-not-exist --format json
run_step unknown_flag status --bogus-flag --format json
run_step unknown_command frobnicate
finish_scenario

begin_scenario contract-surfaces
with_handler
# Precondition: demo-hook registered before the refusal probe.
setup_run add demo-hook --source ./handler.sh
run_step remove_refused remove demo-hook --format json
run_step capabilities_probe capabilities --format json
run_step api_version_ok status --api-version 1.0 --format json
finish_scenario

echo "==> cassettes written to $CASSETTES"
