package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestRoot_StrictValidate_Boots locks in the acceptance gate:
// kit v0.4.0-alpha.2 strict validation must pass at boot. If any leaf
// command loses its kit/side-effect, kit/idempotent, kit/top-level-verb
// annotation or Long help, or if the root exceeds MaxTopLevelVerbs,
// `nerv --help` will exit non-zero with "cli validation failed". This
// test catches the regression.
func TestRoot_StrictValidate_Boots(t *testing.T) {
	bin := buildNerv(t)

	out, err := exec.Command(bin, "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("--help exited non-zero: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "cli validation failed") {
		t.Fatalf("strict validator failed:\n%s", out)
	}

	// Smoke a few subcommand --help paths to make sure the validator
	// permits the full tree, not just the root.
	for _, args := range [][]string{
		{"status"},
		{"install", "--help"},
		{"install", "status", "--help"},
		{"daemon", "--help"},
		{"inbox", "--help"},
		{"inbox", "list", "--help"},
		{"remove", "--help"},
	} {
		out, err := exec.Command(bin, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("nerv %s exited non-zero: %v\n%s", strings.Join(args, " "), err, out)
		}
		if strings.Contains(string(out), "cli validation failed") {
			t.Fatalf("nerv %s tripped validator:\n%s", strings.Join(args, " "), out)
		}
	}
}
