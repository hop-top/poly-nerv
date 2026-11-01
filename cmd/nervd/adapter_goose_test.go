//go:build !windows

package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestAdapterRouting_GooseGolden: real Goose hook payloads (stdin) and
// the exact stdout/exit Goose reads (hooks.md "stdout Is the Decision
// Channel"): block via {"decision":"block","reason":...} on PreToolUse
// and Stop; empty stdout allows; context and warn text are dropped from
// stdout.
func TestAdapterRouting_GooseGolden(t *testing.T) {
	pre := `{"event":"PreToolUse","session_id":"s1","matcher_context":"shell","tool_call_id":"c1","tool_name":"shell","tool_input":{"command":"rm -rf /"},"working_dir":"/w"}`
	stop := `{"event":"Stop","session_id":"s1","last_assistant_message":"done","working_dir":"/w"}`
	post := `{"event":"PostToolUse","session_id":"s1","tool_name":"shell","working_dir":"/w"}`
	start := `{"event":"SessionStart","session_id":"s1","matcher_context":null,"working_dir":"/w"}`
	cases := []struct {
		name, nervEvent, stdin, handler string
		hostExit                        int
		want, wantErr                   string
	}{
		{"block_pretooluse", "PreToolUse", pre, `{"action":"block","message":"no rm"}`, 2,
			`{"decision":"block","reason":"no rm"}`, ""},
		{"block_stop", "Stop", stop, `{"action":"block","message":"run tests first"}`, 2,
			`{"decision":"block","reason":"run tests first"}`, ""},
		{"block_posttooluse_ignored", "PostToolUse", post, `{"action":"block","message":"x"}`, 2, ``, ""},
		{"allow", "PreToolUse", pre, `{"action":"allow"}`, 0, ``, ""},
		{"allow_context_dropped", "SessionStart", start, `{"action":"allow","context":"impact: 3 callers"}`, 0, ``, ""},
		{"warn", "PreToolUse", pre, `{"action":"warn","message":"careful"}`, 1, ``, "nervd: warn: careful"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route, err := lookupAdapter("goose")
			if err != nil || route == nil {
				t.Fatalf("lookupAdapter(goose) = %v, %v", route, err)
			}
			d := ctxDispatcher(t, tc.nervEvent, tc.handler, tc.hostExit)
			var out, errw bytes.Buffer
			code := runStatelessIO(d, route, strings.NewReader(tc.stdin), &out, &errw)
			if code != 0 {
				t.Fatalf("exit = %d, want 0 (block rides stdout); stderr %q", code, errw.String())
			}
			if got := strings.TrimSpace(out.String()); got != tc.want {
				t.Fatalf("stdout\n got %q\nwant %q", got, tc.want)
			}
			if tc.wantErr != "" && !strings.Contains(errw.String(), tc.wantErr) {
				t.Fatalf("stderr = %q, want %q", errw.String(), tc.wantErr)
			}
		})
	}
}

func TestLookupAdapter_GooseListed(t *testing.T) {
	_, err := lookupAdapter("bogus")
	if err == nil || !strings.Contains(err.Error(), "goose") {
		t.Fatalf("supported list missing goose: %v", err)
	}
}
