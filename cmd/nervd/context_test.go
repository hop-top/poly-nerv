package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"hop.top/nerv/internal/merger"
)

func TestRespondClaude_ContextReachesModel(t *testing.T) {
	for _, ev := range []string{"SessionStart", "PreToolUse"} {
		t.Run(ev, func(t *testing.T) {
			var out, errw bytes.Buffer
			code := respondClaude(&out, &errw, ev,
				merger.Decision{Action: merger.Allow, Context: "impact: 3 callers"})
			if code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}
			var m struct {
				HSO map[string]any `json:"hookSpecificOutput"`
			}
			if err := json.Unmarshal(out.Bytes(), &m); err != nil {
				t.Fatalf("unmarshal %q: %v", out.String(), err)
			}
			if m.HSO["hookEventName"] != ev || m.HSO["additionalContext"] != "impact: 3 callers" {
				t.Fatalf("stdout = %q", out.String())
			}
		})
	}
}
