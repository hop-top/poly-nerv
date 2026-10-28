package merger

import "testing"

func TestMerge_AllAllow(t *testing.T) {
	got := Merge([]Decision{
		{Action: Allow},
		{Action: Allow},
		{Action: Allow},
	})
	if got.Action != Allow {
		t.Fatalf("want allow, got %s", got.Action)
	}
}

func TestMerge_BlockWins(t *testing.T) {
	got := Merge([]Decision{
		{Action: Allow},
		{Action: Warn, Message: "hmm"},
		{Action: Block, Message: "nope"},
		{Action: Allow},
	})
	if got.Action != Block {
		t.Fatalf("want block, got %s", got.Action)
	}
}

func TestMerge_FirstBlockMessage(t *testing.T) {
	got := Merge([]Decision{
		{Action: Block, Message: "first"},
		{Action: Block, Message: "second"},
	})
	if got.Message != "first" {
		t.Fatalf("want first block message, got %q", got.Message)
	}
}

func TestMerge_RewriteCompose(t *testing.T) {
	got := Merge([]Decision{
		{Action: Rewrite, Rewrite: map[string]string{"cmd": "ls"}},
		{Action: Rewrite, Rewrite: map[string]string{"cmd": "ls -la"}},
	})
	if got.Action != Rewrite {
		t.Fatalf("want rewrite, got %s", got.Action)
	}
	m, ok := got.Rewrite.(map[string]string)
	if !ok {
		t.Fatalf("want map[string]string, got %T", got.Rewrite)
	}
	if m["cmd"] != "ls -la" {
		t.Fatalf("want ls -la, got %s", m["cmd"])
	}
}

func TestMerge_WarnAccumulate(t *testing.T) {
	got := Merge([]Decision{
		{Action: Warn, Message: "a"},
		{Action: Warn, Message: "b"},
		{Action: Warn, Message: "c"},
	})
	if got.Action != Warn {
		t.Fatalf("want warn, got %s", got.Action)
	}
	if got.Message != "a\nb\nc" {
		t.Fatalf("want concatenated warns, got %q", got.Message)
	}
}

func TestMerge_RewriteOverWarn(t *testing.T) {
	got := Merge([]Decision{
		{Action: Warn, Message: "careful"},
		{Action: Rewrite, Rewrite: map[string]string{"x": "1"}},
	})
	if got.Action != Rewrite {
		t.Fatalf("want rewrite, got %s", got.Action)
	}
	if got.Message != "careful" {
		t.Fatalf("want warn message preserved, got %q", got.Message)
	}
}

func TestMerge_EmptyInput(t *testing.T) {
	got := Merge(nil)
	if got.Action != Allow {
		t.Fatalf("want allow for empty, got %s", got.Action)
	}
}

func TestMerge_SingleDecision(t *testing.T) {
	d := Decision{Action: Warn, Message: "solo"}
	got := Merge([]Decision{d})
	if got.Action != Warn || got.Message != "solo" {
		t.Fatalf("want passthrough, got %s %q", got.Action, got.Message)
	}
}

func TestMerge_PreservesAllSkips(t *testing.T) {
	// Each decision carries a one-entry skips list; Merge must
	// concatenate, not overwrite. Critical regression for the multi-
	// disabled-hook case.
	got := Merge([]Decision{
		{
			Action: Allow,
			Metadata: map[string]any{
				MetaSkips: []map[string]any{
					{"hook": "lint", "event": "PreToolUse", "reason": "disabled-hook"},
				},
			},
		},
		{
			Action: Allow,
			Metadata: map[string]any{
				MetaSkips: []map[string]any{
					{"hook": "audit", "event": "PreToolUse", "reason": "disabled-binding"},
				},
			},
		},
	})
	if got.Action != Allow {
		t.Fatalf("action=%s, want allow", got.Action)
	}
	skips, ok := got.Metadata[MetaSkips].([]map[string]any)
	if !ok {
		t.Fatalf("skips type=%T, want []map[string]any: %+v", got.Metadata[MetaSkips], got.Metadata)
	}
	if len(skips) != 2 {
		t.Fatalf("len(skips)=%d, want 2: %+v", len(skips), skips)
	}
	if skips[0]["hook"] != "lint" {
		t.Errorf("skips[0].hook=%v, want lint", skips[0]["hook"])
	}
	if skips[1]["hook"] != "audit" {
		t.Errorf("skips[1].hook=%v, want audit", skips[1]["hook"])
	}
}

func TestMerge_SkipsCoexistWithBlock(t *testing.T) {
	// Even when a hook blocks, prior skip metadata from earlier
	// decisions must still appear in the merged result so the audit
	// trail doesn't drop on block.
	got := Merge([]Decision{
		{
			Action: Allow,
			Metadata: map[string]any{
				MetaSkips: []map[string]any{
					{"hook": "lint", "reason": "disabled-hook"},
				},
			},
		},
		{Action: Block, Message: "denied"},
	})
	if got.Action != Block {
		t.Fatalf("action=%s, want block", got.Action)
	}
	skips, _ := got.Metadata[MetaSkips].([]map[string]any)
	if len(skips) != 1 {
		t.Fatalf("len(skips)=%d, want 1: %+v", len(skips), got.Metadata)
	}
	if skips[0]["hook"] != "lint" {
		t.Errorf("skips[0].hook=%v, want lint", skips[0]["hook"])
	}
}

func TestMerge_SkipsInputsNotMutated(t *testing.T) {
	// The merger must not mutate the input slice's backing storage
	// (callers reuse decision slices across replay/test fixtures).
	in := []map[string]any{{"hook": "lint"}}
	d1 := Decision{Action: Allow, Metadata: map[string]any{MetaSkips: in}}
	d2 := Decision{Action: Allow, Metadata: map[string]any{MetaSkips: []map[string]any{{"hook": "audit"}}}}
	_ = Merge([]Decision{d1, d2})
	if len(in) != 1 || in[0]["hook"] != "lint" {
		t.Fatalf("input slice mutated: %+v", in)
	}
}
