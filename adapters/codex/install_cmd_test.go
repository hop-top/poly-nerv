package codex

import (
	"strings"
	"testing"
)

func TestMergeConfig_EmitsAdapterFlag(t *testing.T) {
	out, err := New().MergeConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if n := strings.Count(s, `"command": "nervd --adapter codex"`); n != 5 {
		t.Fatalf("want 5 flagged commands, got %d:\n%s", n, s)
	}
	if strings.Contains(s, "nerv dispatch") {
		t.Fatalf("legacy command present:\n%s", s)
	}
}

func TestMergeConfig_UpgradesLegacyCommand(t *testing.T) {
	legacy := []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"nerv dispatch"}]}]}}`)
	out, err := New().MergeConfig(legacy)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "nerv dispatch") {
		t.Fatalf("legacy not upgraded:\n%s", s)
	}
	if n := strings.Count(s, "nervd --adapter codex"); n != 5 {
		t.Fatalf("want 5, got %d", n)
	}
}
