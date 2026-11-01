package vibe

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
	if n := strings.Count(s, `command = "nervd --adapter vibe"`); n != 6 {
		t.Fatalf("want 6 flagged commands, got %d:\n%s", n, s)
	}
	if strings.Contains(s, `command = "nervd"`) {
		t.Fatalf("bare nervd command present:\n%s", s)
	}
}

func TestMergeConfig_UpgradesLegacyBareCommand(t *testing.T) {
	legacy := []byte("[hooks]\nenabled = true\n\n[[hooks.handlers]]\nevent = \"ToolResultEvent\"\ntype = \"command\"\ncommand = \"nervd\"\n")
	out, err := (&Adapter{}).MergeConfig(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `command = "nervd --adapter vibe"`) ||
		strings.Contains(string(out), `command = "nervd"`) {
		t.Fatalf("legacy command not upgraded:\n%s", out)
	}
}
