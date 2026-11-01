//go:build !windows

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/dispatch"
)

// ctxDispatcher returns a dispatcher whose single hook on nerv event
// onEvent allows with the given action JSON.
func ctxDispatcher(t *testing.T, onEvent, body string, exit int) *dispatch.Dispatcher {
	t.Helper()
	script := filepath.Join(t.TempDir(), "h.sh")
	src := "#!/bin/sh\ncat >/dev/null\necho '" + body + "'\nexit " + string(rune('0'+exit)) + "\n"
	if err := os.WriteFile(script, []byte(src), 0o700); err != nil {
		t.Fatal(err)
	}
	cc := config.Compile(&config.HooksConfig{Hooks: []config.HookEntry{
		{Name: "h", On: onEvent, Run: script},
	}})
	return dispatch.New(cc, nil, dispatch.NewMatcher(), dispatch.NewRunner())
}

const ctxAllow = `{"action":"allow","context":"impact: 3 callers"}`

// TestAdapterRouting_ContextGolden: one context-bearing event per
// adapter; golden stdout and exit code through --adapter routing.
func TestAdapterRouting_ContextGolden(t *testing.T) {
	cases := []struct {
		adapter, nervEvent, stdin, want string
	}{
		{"codex", "SessionStart",
			`{"hook_event_name":"SessionStart","session_id":"s1"}`,
			`{"hookSpecificOutput":{"additionalContext":"impact: 3 callers","hookEventName":"SessionStart"}}`},
		{"gemini", "SessionStart",
			`{"hook_event_name":"SessionStart","session_id":"s1"}`,
			`{"decision":"allow","hookSpecificOutput":{"additionalContext":"impact: 3 callers"}}`},
		{"openhands", "SessionStart",
			`{"hook_event_name":"SessionStart","session_id":"s1"}`,
			`{"action":"allow","additionalContext":"impact: 3 callers"}`},
		{"vibe", "PostToolUse",
			`{"event":"ToolResultEvent","session_id":"s1"}`,
			`{"decision":"allow","hook_specific_output":{"additional_context":"impact: 3 callers"}}`},
		{"opencode", "SessionStart",
			`{"type":"session.created","session_id":"s1"}`,
			`{"action":"allow","context":"impact: 3 callers"}`},
	}
	for _, tc := range cases {
		t.Run(tc.adapter, func(t *testing.T) {
			route, err := lookupAdapter(tc.adapter)
			if err != nil || route == nil {
				t.Fatalf("lookupAdapter: %v %v", route, err)
			}
			d := ctxDispatcher(t, tc.nervEvent, ctxAllow, 0)
			var out, errw bytes.Buffer
			code := runStatelessIO(d, route, strings.NewReader(tc.stdin), &out, &errw)
			if code != 0 {
				t.Fatalf("exit = %d, stderr %q", code, errw.String())
			}
			if got := strings.TrimSpace(out.String()); got != tc.want {
				t.Fatalf("stdout\n got %s\nwant %s", got, tc.want)
			}
		})
	}
}

// TestAdapterRouting_BlockExit: block keeps host exit semantics and
// drops context.
func TestAdapterRouting_BlockExit(t *testing.T) {
	route, _ := lookupAdapter("gemini")
	d := ctxDispatcher(t, "PreToolUse",
		`{"action":"block","message":"no","context":"x"}`, 2)
	var out, errw bytes.Buffer
	code := runStatelessIO(d, route,
		strings.NewReader(`{"hook_event_name":"BeforeTool","session_id":"s1"}`), &out, &errw)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if strings.Contains(out.String(), "additionalContext") ||
		!strings.Contains(out.String(), `"decision":"deny"`) {
		t.Fatalf("stdout = %q", out.String())
	}
}

// TestAdapterRouting_NoFlagStaysClaude: without --adapter, hook_event_name
// input is answered in the claude dialect, as before.
func TestAdapterRouting_NoFlagStaysClaude(t *testing.T) {
	route, err := lookupAdapter("")
	if err != nil || route != nil {
		t.Fatalf("lookupAdapter(\"\") = %v, %v; want nil, nil", route, err)
	}
	d := ctxDispatcher(t, "SessionStart", ctxAllow, 0)
	var out, errw bytes.Buffer
	code := runStatelessIO(d, nil,
		strings.NewReader(`{"hook_event_name":"SessionStart","session_id":"s1"}`), &out, &errw)
	want := `{"hookSpecificOutput":{"additionalContext":"impact: 3 callers","hookEventName":"SessionStart"}}`
	if code != 0 || strings.TrimSpace(out.String()) != want {
		t.Fatalf("code %d stdout %q", code, out.String())
	}
	// Explicit claude is the same path.
	if r, err := lookupAdapter("claude"); err != nil || r != nil {
		t.Fatalf("claude: %v %v", r, err)
	}
}

func TestLookupAdapter_Unknown(t *testing.T) {
	_, err := lookupAdapter("bogus")
	if err == nil || !strings.Contains(err.Error(), `unknown adapter "bogus"`) ||
		!strings.Contains(err.Error(), "codex") {
		t.Fatalf("err = %v", err)
	}
}

// TestNervdBinary_UnknownAdapter: the built binary rejects an unknown
// --adapter before touching stdin or config: exit 2, message on stderr.
func TestNervdBinary_UnknownAdapter(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "nervd")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, "--adapter", "bogus")
	cmd.Stdin = strings.NewReader("{}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	ee, ok := err.(*exec.ExitError)
	if !ok || ee.ExitCode() != 2 {
		t.Fatalf("err = %v, want exit 2", err)
	}
	if !strings.Contains(stderr.String(), `unknown adapter "bogus"`) || stdout.Len() != 0 {
		t.Fatalf("stderr %q stdout %q", stderr.String(), stdout.String())
	}
}
