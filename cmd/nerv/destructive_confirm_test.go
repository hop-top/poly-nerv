package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDestructiveCommands_RefuseInNonTTY locks in kit's --confirm policy
// for nerv's destructive surface. In non-TTY (which `exec.Command` always
// is), commands tagged `kit/side-effect: destructive` must refuse unless
// --confirm=yes is passed. Regression test; if either command
// stops carrying the destructive tag or kit changes the contract, this
// catches it.
func TestDestructiveCommands_RefuseInNonTTY(t *testing.T) {
	bin := buildNerv(t)

	cases := []struct {
		name string
		args []string
	}{
		{"remove", []string{"remove", "some-hook"}},
		{"daemon_stop", []string{"daemon", "stop"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := exec.Command(bin, tc.args...).CombinedOutput()
			if err == nil {
				t.Fatalf("expected non-zero exit; got success. output:\n%s", out)
			}
			s := string(out)
			if !strings.Contains(s, "UNAUTHORIZED") || !strings.Contains(s, "--confirm") {
				t.Fatalf("expected UNAUTHORIZED/--confirm refusal; got:\n%s", s)
			}
		})
	}
}

// TestDestructiveCommands_AcceptWithConfirmYes verifies the escape hatch:
// with --confirm=yes, kit's policy middleware lets the command through.
// We can't run `remove some-hook` to completion in a test (it would touch
// real state); --dry-run is the standard preview path for destructive
// commands per CLI conventions §3.6.
func TestDestructiveCommands_AcceptWithConfirmYes(t *testing.T) {
	bin := buildNerv(t)

	// --dry-run + --confirm=yes should pass the policy gate.
	out, err := exec.Command(bin, "remove", "some-hook", "--confirm=yes", "--dry-run").CombinedOutput()
	s := string(out)
	if strings.Contains(s, "UNAUTHORIZED") {
		t.Fatalf("did not expect UNAUTHORIZED with --confirm=yes; got:\n%s", s)
	}
	_ = err // dry-run may still error on missing hook; we only assert the policy gate passed
}

func buildNerv(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "nerv-test")
	out, err := exec.Command("go", "build", "-buildvcs=false", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}
