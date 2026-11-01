package openhands

import (
	"strings"
	"testing"
)

func TestMergeConfig_EmitsAdapterFlag(t *testing.T) {
	out, err := (&Adapter{}).MergeConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if n := strings.Count(s, `"command": "nervd --adapter openhands"`); n != 6 {
		t.Fatalf("want 6 flagged commands, got %d:\n%s", n, s)
	}
	if strings.Contains(s, `"command": "nervd"`) {
		t.Fatalf("bare nervd command present:\n%s", s)
	}
}

// A legacy install (bare nervd) is upgraded in place, not duplicated.
func TestMergeConfig_UpgradesLegacyBareCommand(t *testing.T) {
	legacy := []byte(`{"hooks":{"SessionStart":[{"matcher":".*","hooks":[{"type":"command","command":"nervd","timeout":30000}]}]}}`)
	out, err := (&Adapter{}).MergeConfig(legacy)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, `"command": "nervd"`) {
		t.Fatalf("legacy command not upgraded:\n%s", s)
	}
	if n := strings.Count(s, "nervd --adapter openhands"); n != 6 {
		t.Fatalf("want 6 flagged commands (1 upgraded + 5 new), got %d", n)
	}
}
