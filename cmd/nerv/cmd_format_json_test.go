package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMutatingLeaves_EmitJSONUnderFormatJSON locks in factor-3
// structured I/O for the leaves that previously ignored --format:
// `add` printed `hook "…" installed from …`, `remove` printed
// `hook "…" removed`, and `daemon status` printed a bare `stopped`.
// Under --format json every leaf must put parseable JSON on stdout.
func TestMutatingLeaves_EmitJSONUnderFormatJSON(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()
	writeHandler(t, project)

	assertJSON := func(name, stdout string) {
		t.Helper()
		var v any
		if err := json.Unmarshal([]byte(stdout), &v); err != nil {
			t.Errorf("%s: stdout under --format json is not JSON: %v\nstdout:\n%s",
				name, err, stdout)
		}
	}

	// add (real apply).
	stdout, stderr, code := runInProject(t, bin, project,
		"add", "demo-hook", "--source", "./handler.sh", "--format", "json")
	if code != 0 {
		t.Fatalf("add exited %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertJSON("add", stdout)
	if !strings.Contains(stdout, "demo-hook") {
		t.Errorf("add JSON does not identify the hook:\n%s", stdout)
	}

	// remove (real apply, confirm gate satisfied).
	stdout, stderr, code = runInProject(t, bin, project,
		"remove", "demo-hook", "--confirm=yes", "--format", "json")
	if code != 0 {
		t.Fatalf("remove exited %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertJSON("remove", stdout)

	// daemon status (read leaf, daemon stopped in the sandbox).
	stdout, stderr, code = runInProject(t, bin, project,
		"daemon", "status", "--format", "json")
	if code != 0 {
		t.Fatalf("daemon status exited %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	assertJSON("daemon status", stdout)
	if !strings.Contains(stdout, "stopped") {
		t.Errorf("daemon status JSON does not report state:\n%s", stdout)
	}
}
