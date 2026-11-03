//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Both handler rewrite forms reach the host as the bare tool input,
// through nervd stateless on every adapter that honors rewrite.
func TestStateless_RewriteForms(t *testing.T) {
	forms := map[string]string{
		"bare":      `{"action":"rewrite","rewrite":{"command":"ls -la"}}`,
		"canonical": `{"action":"rewrite","rewrite":{"tool_input":{"command":"ls -la"}}}`,
	}
	adapters := []struct {
		name, stdin string
		check       func(t *testing.T, out map[string]any)
	}{
		{"claude", `{"hook_event_name":"PreToolUse","session_id":"s1","tool_name":"Bash","tool_input":{"command":"ls"}}`,
			func(t *testing.T, out map[string]any) {
				hso, _ := out["hookSpecificOutput"].(map[string]any)
				wantInput(t, hso["updatedInput"])
			}},
		{"codex", `{"hook_event_name":"PreToolUse","session_id":"s1","tool_name":"Bash","tool_input":{"command":"ls"}}`,
			func(t *testing.T, out map[string]any) {
				hso, _ := out["hookSpecificOutput"].(map[string]any)
				wantInput(t, hso["updatedInput"])
			}},
		{"gemini", `{"hook_event_name":"BeforeTool","session_id":"s1"}`,
			func(t *testing.T, out map[string]any) {
				hso, _ := out["hookSpecificOutput"].(map[string]any)
				wantInput(t, hso["tool_input"])
			}},
		{"opencode", `{"type":"tool.execute.before","session_id":"s1"}`,
			func(t *testing.T, out map[string]any) {
				wantInput(t, out["rewrite"])
			}},
	}
	for form, body := range forms {
		for _, a := range adapters {
			t.Run(form+"/"+a.name, func(t *testing.T) {
				var route *adapterRoute
				if a.name != "claude" {
					r, err := lookupAdapter(a.name)
					if err != nil {
						t.Fatal(err)
					}
					route = r
				}
				d := ctxDispatcher(t, "PreToolUse", body, 3)
				var out, errw bytes.Buffer
				runStatelessIO(d, route, strings.NewReader(a.stdin), &out, &errw)
				var m map[string]any
				if err := json.Unmarshal(out.Bytes(), &m); err != nil {
					t.Fatalf("stdout %q stderr %q: %v", out.String(), errw.String(), err)
				}
				a.check(t, m)
			})
		}
	}
}

func wantInput(t *testing.T, got any) {
	t.Helper()
	m, ok := got.(map[string]any)
	if !ok || len(m) != 1 || m["command"] != "ls -la" {
		t.Fatalf("host tool input = %#v, want exactly {command: ls -la}", got)
	}
}
