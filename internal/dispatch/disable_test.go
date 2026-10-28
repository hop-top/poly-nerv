package dispatch

import (
	"testing"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/merger"
)

// compileWithSidecar overlays a sidecar on top of compileCfg so tests
// can exercise the disable filter without writing files.
func compileWithSidecar(sidecar *config.Disabled, hooks ...config.HookEntry) *config.CachedConfig {
	cc := compileCfg(hooks...)
	cc.Disabled = sidecar
	return cc
}

// skipsOf returns the metadata.skips list from a decision, or nil if
// absent. Tolerates both []map[string]any (in-process) and []any
// (round-tripped through JSON).
func skipsOf(d *merger.Decision) []map[string]any {
	if d == nil || d.Metadata == nil {
		return nil
	}
	switch v := d.Metadata[MetaSkips].(type) {
	case []map[string]any:
		return v
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, it := range v {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// findSkip returns the first skip entry whose reason matches want,
// across the merged metadata.skips list of a decision. Returns nil if
// no entry matches.
func findSkip(d *merger.Decision, want string) map[string]any {
	for _, s := range skipsOf(d) {
		if r, _ := s[SkipReason].(string); r == want {
			return s
		}
	}
	return nil
}

// findSkipInSlice scans a pre-merge slice of skip decisions (each with
// a one-entry skips list) for the first one whose reason matches want.
func findSkipInSlice(decisions []merger.Decision, want string) map[string]any {
	for i := range decisions {
		if entry := findSkip(&decisions[i], want); entry != nil {
			return entry
		}
	}
	return nil
}

func TestDispatch_SkipsDisabledHook(t *testing.T) {
	// Hook with Disabled=true must not run; merger sees only the
	// synthesised skip decision; event log records it.
	cfg := compileWithSidecar(nil,
		config.HookEntry{Name: "lint", On: "PreToolUse", Run: "false", Disabled: true},
	)
	lg := &mockLogger{}
	d := New(cfg, lg, NewMatcher(), NewRunner())

	res, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	// No live hooks → merger returns Allow.
	if res.Decision.Action != merger.Allow {
		t.Fatalf("action = %s, want allow", res.Decision.Action)
	}
	// Skip metadata propagates through merger as a one-entry list.
	skips := skipsOf(&res.Decision)
	if len(skips) != 1 {
		t.Fatalf("skips=%d, want 1: %+v", len(skips), res.Decision.Metadata)
	}
	if skips[0][SkipReason] != ReasonDisabledHook {
		t.Errorf("reason = %v, want %s", skips[0][SkipReason], ReasonDisabledHook)
	}
	if skips[0][SkipHook] != "lint" {
		t.Errorf("hook = %v, want lint", skips[0][SkipHook])
	}
	if len(lg.entries) != 1 {
		t.Fatalf("logged %d entries, want 1", len(lg.entries))
	}
}

func TestDispatch_SkipsDisabledEvent(t *testing.T) {
	// Sidecar disables the entire event; both hooks skip. The merged
	// decision must surface BOTH skip entries (this is the bug fix).
	sidecar := &config.Disabled{Events: []string{"PreToolUse"}}
	cfg := compileWithSidecar(sidecar,
		config.HookEntry{Name: "lint", On: "PreToolUse", Run: "false"},
		config.HookEntry{Name: "audit", On: "PreToolUse", Run: "false"},
	)
	lg := &mockLogger{}
	d := New(cfg, lg, NewMatcher(), NewRunner())

	res, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if res.Decision.Action != merger.Allow {
		t.Fatalf("action = %s, want allow", res.Decision.Action)
	}
	skips := skipsOf(&res.Decision)
	if len(skips) != 2 {
		t.Fatalf("skips=%d, want 2: %+v", len(skips), res.Decision.Metadata)
	}
	// Both entries must report disabled-event, one per hook.
	got := map[string]string{}
	for _, s := range skips {
		hook, _ := s[SkipHook].(string)
		reason, _ := s[SkipReason].(string)
		got[hook] = reason
	}
	if got["lint"] != ReasonDisabledEvent {
		t.Errorf("lint reason = %v, want %s", got["lint"], ReasonDisabledEvent)
	}
	if got["audit"] != ReasonDisabledEvent {
		t.Errorf("audit reason = %v, want %s", got["audit"], ReasonDisabledEvent)
	}
}

func TestDispatch_SkipsDisabledBinding(t *testing.T) {
	// Only the (lint, PreToolUse) pair is disabled; audit still runs.
	sidecar := &config.Disabled{
		Bindings: []config.DisabledBinding{
			{Hook: "lint", Event: "PreToolUse"},
		},
	}
	cfg := compileWithSidecar(sidecar,
		config.HookEntry{Name: "lint", On: "PreToolUse", Run: "true"},
		config.HookEntry{Name: "audit", On: "PreToolUse", Run: "true"},
	)
	lg := &mockLogger{}
	d := New(cfg, lg, NewMatcher(), NewRunner())

	res, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if res.Decision.Action != merger.Allow {
		t.Fatalf("action = %s, want allow", res.Decision.Action)
	}
	skips := skipsOf(&res.Decision)
	if len(skips) != 1 {
		t.Fatalf("skips=%d, want 1: %+v", len(skips), res.Decision.Metadata)
	}
	if skips[0][SkipReason] != ReasonDisabledBinding {
		t.Errorf("reason = %v, want %s", skips[0][SkipReason], ReasonDisabledBinding)
	}
	if skips[0][SkipHook] != "lint" {
		t.Errorf("skipped hook = %v, want lint", skips[0][SkipHook])
	}
}

func TestDispatch_SkipsMultipleHooksOnSameEnvelope(t *testing.T) {
	// Critical regression: 2+ disabled hooks on the same envelope must
	// each surface as their own skip entry in the merged decision.
	// Pre-fix: only the LAST skip's metadata survived (top-level keys
	// overwrote each other). Post-fix: a metadata.skips list preserves
	// every entry.
	sidecar := &config.Disabled{
		Bindings: []config.DisabledBinding{
			{Hook: "audit", Event: "PreToolUse"},
		},
	}
	cfg := compileWithSidecar(sidecar,
		config.HookEntry{Name: "lint", On: "PreToolUse", Run: "false", Disabled: true},
		config.HookEntry{Name: "audit", On: "PreToolUse", Run: "false"},
		config.HookEntry{Name: "trace", On: "PreToolUse", Run: "true"},
	)
	lg := &mockLogger{}
	d := New(cfg, lg, NewMatcher(), NewRunner())

	res, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if res.Decision.Action != merger.Allow {
		t.Fatalf("action = %s, want allow (trace was a no-op)", res.Decision.Action)
	}
	skips := skipsOf(&res.Decision)
	if len(skips) != 2 {
		t.Fatalf("skips=%d, want 2 (lint + audit): %+v", len(skips), res.Decision.Metadata)
	}
	// Inspect by (hook, reason).
	got := map[string]string{}
	for _, s := range skips {
		hook, _ := s[SkipHook].(string)
		reason, _ := s[SkipReason].(string)
		got[hook] = reason
	}
	if got["lint"] != ReasonDisabledHook {
		t.Errorf("lint reason = %v, want %s", got["lint"], ReasonDisabledHook)
	}
	if got["audit"] != ReasonDisabledBinding {
		t.Errorf("audit reason = %v, want %s", got["audit"], ReasonDisabledBinding)
	}
}

func TestSplitDisabled_OnlyMatchedSkip(t *testing.T) {
	// Sanity: with a binding that doesn't match anything, every hook
	// still runs.
	sidecar := &config.Disabled{
		Bindings: []config.DisabledBinding{
			{Hook: "lint", Event: "OtherEvent"},
		},
	}
	hooks := []config.HookEntry{
		{Name: "lint", On: "PreToolUse"},
		{Name: "audit", On: "PreToolUse"},
	}
	live, skipped := splitDisabled(hooks, newTestEnvelope("PreToolUse"), sidecar)
	if len(live) != 2 {
		t.Fatalf("live=%d, want 2", len(live))
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped=%d, want 0", len(skipped))
	}
}

func TestSplitDisabled_MixedHookAndBinding(t *testing.T) {
	// Lint disabled at the hook level; audit disabled at the binding
	// level; check both surface as skip decisions with distinct
	// reasons.
	sidecar := &config.Disabled{
		Bindings: []config.DisabledBinding{
			{Hook: "audit", Event: "PreToolUse"},
		},
	}
	hooks := []config.HookEntry{
		{Name: "lint", On: "PreToolUse", Disabled: true},
		{Name: "audit", On: "PreToolUse"},
		{Name: "trace", On: "PreToolUse"},
	}
	live, skipped := splitDisabled(hooks, newTestEnvelope("PreToolUse"), sidecar)
	if len(live) != 1 || live[0].Name != "trace" {
		t.Fatalf("live=%v, want [trace]", live)
	}
	if len(skipped) != 2 {
		t.Fatalf("skipped=%d, want 2", len(skipped))
	}
	if findSkipInSlice(skipped, ReasonDisabledHook) == nil {
		t.Errorf("missing disabled-hook skip in %+v", skipped)
	}
	if findSkipInSlice(skipped, ReasonDisabledBinding) == nil {
		t.Errorf("missing disabled-binding skip in %+v", skipped)
	}
}

func TestSplitDisabled_HookFlagBeatsBinding(t *testing.T) {
	// When both the per-hook flag and a binding match the same hook,
	// the per-hook reason should win (cheapest check, evaluated first).
	sidecar := &config.Disabled{
		Bindings: []config.DisabledBinding{
			{Hook: "lint", Event: "PreToolUse"},
		},
	}
	hooks := []config.HookEntry{
		{Name: "lint", On: "PreToolUse", Disabled: true},
	}
	_, skipped := splitDisabled(hooks, newTestEnvelope("PreToolUse"), sidecar)
	if len(skipped) != 1 {
		t.Fatalf("skipped=%d, want 1", len(skipped))
	}
	entry := findSkipInSlice(skipped, ReasonDisabledHook)
	if entry == nil {
		t.Fatalf("missing disabled-hook entry in %+v", skipped)
	}
}

func TestSplitDisabled_NilSidecar(t *testing.T) {
	hooks := []config.HookEntry{
		{Name: "lint", On: "PreToolUse"},
		{Name: "audit", On: "PreToolUse", Disabled: true},
	}
	live, skipped := splitDisabled(hooks, newTestEnvelope("PreToolUse"), nil)
	if len(live) != 1 || live[0].Name != "lint" {
		t.Fatalf("live=%v, want [lint]", live)
	}
	if len(skipped) != 1 {
		t.Fatalf("skipped=%d, want 1", len(skipped))
	}
}

func TestDispatcher_ReloadPicksUpChanges(t *testing.T) {
	// Dispatcher starts with nothing disabled; first dispatch runs the
	// hook. We then Reload with a sidecar that disables the event, and
	// the second dispatch must surface the skip.
	cfg := compileWithSidecar(nil,
		config.HookEntry{Name: "lint", On: "PreToolUse", Run: "true"},
	)
	d := New(cfg, nil, NewMatcher(), NewRunner())

	res, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatalf("dispatch (pre-reload): %v", err)
	}
	if skipsOf(&res.Decision) != nil {
		t.Fatalf("pre-reload should not be a skip, got skips=%+v", skipsOf(&res.Decision))
	}

	// Swap in a sidecar that disables the event.
	swapped := compileWithSidecar(&config.Disabled{Events: []string{"PreToolUse"}},
		config.HookEntry{Name: "lint", On: "PreToolUse", Run: "true"},
	)
	d.Reload(swapped)

	res2, err := d.Dispatch(newTestEnvelope("PreToolUse"))
	if err != nil {
		t.Fatalf("dispatch (post-reload): %v", err)
	}
	skips := skipsOf(&res2.Decision)
	if len(skips) != 1 || skips[0][SkipReason] != ReasonDisabledEvent {
		t.Fatalf("post-reload skips=%+v, want one disabled-event entry", skips)
	}
}

func TestDispatcher_ConfigAccessor(t *testing.T) {
	cfg := compileCfg()
	d := New(cfg, nil, NewMatcher(), NewRunner())
	if got := d.Config(); got != cfg {
		t.Fatalf("Config() returned %p, want %p", got, cfg)
	}
}
