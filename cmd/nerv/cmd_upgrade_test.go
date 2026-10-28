package main

import (
	"bytes"
	"strings"
	"testing"
)

// nerv does not self-update; `nerv upgrade` must say so instead of
// claiming to upgrade.
func TestUpgradeCmd_DoesNotClaimSelfUpdate(t *testing.T) {
	cmd := upgradeCmd()

	if strings.Contains(cmd.Short, "Upgrade nerv to the latest version") {
		t.Errorf("Short claims self-update: %q", cmd.Short)
	}
	if got := cmd.Annotations["kit/side-effect"]; got != "read" {
		t.Errorf("kit/side-effect = %q, want read (nothing is written)", got)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	s := out.String()
	for _, want := range []string{
		"does not update itself",
		"go install hop.top/nerv/cmd/nerv@latest",
		"https://github.com/hop-top/poly-nerv/releases",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
}
