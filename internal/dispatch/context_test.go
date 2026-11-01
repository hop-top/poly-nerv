//go:build !windows

package dispatch

import (
	"strings"
	"testing"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/merger"
)

// TestRunHooks_ContextParsed: the optional "context" stdout field
// reaches the decision for every action.
func TestRunHooks_ContextParsed(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		action merger.Action
	}{
		{"allow", `echo '{"action":"allow","context":"ctx-allow"}'; exit 0`, merger.Allow},
		{"warn", `echo '{"action":"warn","message":"m","context":"ctx-warn"}'; exit 1`, merger.Warn},
		{"block", `echo '{"action":"block","message":"m","context":"ctx-block"}'; exit 2`, merger.Block},
		{"rewrite", `echo '{"action":"rewrite","rewrite":{"a":1},"context":"ctx-rewrite"}'; exit 3`, merger.Rewrite},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := writeScript(t, t.TempDir(), "h.sh", "#!/bin/sh\n"+tc.body+"\n")
			ds := NewRunner().RunHooks(
				[]config.HookEntry{{Name: "h", Run: script}}, testEnvelope(), testConfig())
			if len(ds) != 1 {
				t.Fatalf("want 1 decision, got %d", len(ds))
			}
			if ds[0].Action != tc.action {
				t.Fatalf("want %s, got %s", tc.action, ds[0].Action)
			}
			if want := "ctx-" + tc.name; ds[0].Context != want {
				t.Fatalf("want context %q, got %q", want, ds[0].Context)
			}
		})
	}
}

// TestDispatch_ContextMergedAcrossHooks runs real handlers through the
// dispatcher: contexts concatenate in hook order; a block drops them.
func TestDispatch_ContextMergedAcrossHooks(t *testing.T) {
	dir := t.TempDir()
	a := writeScript(t, dir, "a.sh", "#!/bin/sh\necho '{\"action\":\"allow\",\"context\":\"from-a\"}'\n")
	w := writeScript(t, dir, "w.sh", "#!/bin/sh\necho '{\"action\":\"warn\",\"message\":\"hm\",\"context\":\"from-w\"}'\nexit 1\n")
	b := writeScript(t, dir, "b.sh", "#!/bin/sh\necho '{\"action\":\"block\",\"message\":\"no\"}'\nexit 2\n")

	d := New(compileCfg(
		config.HookEntry{Name: "a", On: "PreToolUse", Run: a},
		config.HookEntry{Name: "w", On: "PreToolUse", Run: w},
	), nil, NewMatcher(), NewRunner())
	res, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision.Action != merger.Warn || res.Decision.Context != "from-a\nfrom-w" {
		t.Fatalf("got %s %q", res.Decision.Action, res.Decision.Context)
	}

	d = New(compileCfg(
		config.HookEntry{Name: "a", On: "PreToolUse", Run: a},
		config.HookEntry{Name: "b", On: "PreToolUse", Run: b},
	), nil, NewMatcher(), NewRunner())
	res, err = d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision.Action != merger.Block || res.Decision.Context != "" {
		t.Fatalf("block must drop context, got %s %q", res.Decision.Action, res.Decision.Context)
	}
}

// TestDispatch_ContextCapped: oversized handler context is capped
// with the truncation marker.
func TestDispatch_ContextCapped(t *testing.T) {
	dir := t.TempDir()
	big := writeScript(t, dir, "big.sh",
		"#!/bin/sh\nprintf '{\"action\":\"allow\",\"context\":\"%s\"}' \"$(head -c 20000 /dev/zero | tr '\\0' x)\"\n")
	d := New(compileCfg(
		config.HookEntry{Name: "big", On: "PreToolUse", Run: big},
	), nil, NewMatcher(), NewRunner())
	res, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatal(err)
	}
	c := res.Decision.Context
	if len(c) > merger.MaxContextBytes || !strings.HasSuffix(c, merger.ContextTruncatedMarker) {
		t.Fatalf("want capped context with marker, got %d bytes", len(c))
	}
}
