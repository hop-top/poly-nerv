package main

import (
	"strings"
	"testing"
)

// TestExitCodes_HonorClassTable locks in the kit exit-class contract
// (OK=0, GENERIC=1, USAGE=2, NOT_FOUND=3, UNAUTHORIZED=5): the
// process exit code must match the class the failure belongs to —
// and when a structured error envelope declares an exit_code, the
// process must exit with exactly that code instead of collapsing
// everything to 1.
func TestExitCodes_HonorClassTable(t *testing.T) {
	bin := buildNerv(t)

	cases := []struct {
		name string
		args []string
		want int
	}{
		// Destructive refusal envelope declares "exit_code": 5.
		{"destructive_refusal", []string{"remove", "demo-hook", "--format", "json"}, 5},
		// Unknown flag is a usage error.
		{"unknown_flag", []string{"status", "--bogus-flag", "--format", "json"}, 2},
		// Unknown command is a usage error (previously printed root
		// help and exited 0).
		{"unknown_command", []string{"frobnicate"}, 2},
		// Replay of an unrecorded event is a not-found error.
		{"missing_event", []string{"replay", "evt-does-not-exist", "--format", "json"}, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			stdout, stderr, code := runInProject(t, bin, project, tc.args...)
			if code != tc.want {
				t.Errorf("nerv %s exited %d, want %d\nstdout:\n%s\nstderr:\n%s",
					strings.Join(tc.args, " "), code, tc.want, stdout, stderr)
			}
		})
	}
}

// TestUnknownCommand_ReportsUsageOnStderr pins the diagnostics half
// of the unknown-command fix: the failure surfaces on stderr, not as
// root help on stdout with a clean exit.
func TestUnknownCommand_ReportsUsageOnStderr(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()

	stdout, stderr, code := runInProject(t, bin, project, "frobnicate")
	if code != 2 {
		t.Fatalf("exit %d, want 2\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "frobnicate") {
		t.Errorf("stderr does not name the unknown command:\n%s", stderr)
	}
}

// TestBareRoot_PrintsHelpExitsZero guards the UX the unknown-command
// fix must not regress: a bare `nerv` still prints help and exits 0.
func TestBareRoot_PrintsHelpExitsZero(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()

	stdout, _, code := runInProject(t, bin, project)
	if code != 0 {
		t.Fatalf("bare nerv exited %d, want 0", code)
	}
	if !strings.Contains(stdout, "USAGE") && !strings.Contains(stdout, "Usage") {
		t.Errorf("bare nerv did not print help:\n%s", stdout)
	}
}
