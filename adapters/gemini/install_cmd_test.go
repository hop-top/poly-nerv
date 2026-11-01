package gemini

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
	if n := strings.Count(s, `"command": "nervd --adapter gemini"`); n != 11 {
		t.Fatalf("want 11 flagged commands, got %d:\n%s", n, s)
	}
	if strings.Contains(s, "nerv-adapter") {
		t.Fatalf("legacy command present:\n%s", s)
	}
}

func TestMergeConfig_UpgradesLegacyCommand(t *testing.T) {
	legacy := []byte(`{"hooks":{"AfterTool":[{"matcher":".*","hooks":[{"type":"command","command":"nerv-adapter gemini AfterTool","name":"nerv-AfterTool"}]}]}}`)
	out, err := (&Adapter{}).MergeConfig(legacy)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "nerv-adapter") {
		t.Fatalf("legacy not upgraded:\n%s", s)
	}
	if n := strings.Count(s, "nervd --adapter gemini"); n != 11 {
		t.Fatalf("want 11 (no duplicate for AfterTool), got %d", n)
	}
}
