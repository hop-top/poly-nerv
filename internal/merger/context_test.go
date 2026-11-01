package merger

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMerge_ContextConcatInHookOrder(t *testing.T) {
	got := Merge([]Decision{
		{Action: Allow, Context: "first"},
		{Action: Allow},
		{Action: Allow, Context: "second"},
	})
	if got.Action != Allow {
		t.Fatalf("want allow, got %s", got.Action)
	}
	if got.Context != "first\nsecond" {
		t.Fatalf("want concatenated context, got %q", got.Context)
	}
}

func TestMerge_ContextKeptAcrossWarnAndRewrite(t *testing.T) {
	got := Merge([]Decision{
		{Action: Allow, Context: "a"},
		{Action: Warn, Message: "careful", Context: "b"},
		{Action: Rewrite, Rewrite: map[string]any{"x": 1}, Context: "c"},
	})
	if got.Action != Rewrite {
		t.Fatalf("want rewrite, got %s", got.Action)
	}
	if got.Context != "a\nb\nc" {
		t.Fatalf("want a\\nb\\nc, got %q", got.Context)
	}
	if got.Message != "careful" {
		t.Fatalf("warn message lost: %q", got.Message)
	}

	w := Merge([]Decision{
		{Action: Warn, Message: "m", Context: "w1"},
		{Action: Allow, Context: "w2"},
	})
	if w.Action != Warn || w.Context != "w1\nw2" {
		t.Fatalf("warn merge: got %s %q", w.Action, w.Context)
	}
}

func TestMerge_ContextDroppedOnBlock(t *testing.T) {
	for name, ds := range map[string][]Decision{
		"block-after-context": {
			{Action: Allow, Context: "before"},
			{Action: Block, Message: "no", Context: "own"},
			{Action: Allow, Context: "after"},
		},
		"block-first": {
			{Action: Block, Message: "no"},
			{Action: Warn, Message: "w", Context: "later"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := Merge(ds)
			if got.Action != Block {
				t.Fatalf("want block, got %s", got.Action)
			}
			if got.Context != "" {
				t.Fatalf("context must be dropped on block, got %q", got.Context)
			}
		})
	}
}

func TestMerge_ContextEmptyIsNoop(t *testing.T) {
	got := Merge([]Decision{{Action: Allow}, {Action: Warn, Message: "m"}})
	if got.Context != "" {
		t.Fatalf("want empty context, got %q", got.Context)
	}
}

func TestMerge_ContextCappedWithMarker(t *testing.T) {
	big := strings.Repeat("x", MaxContextBytes)
	got := Merge([]Decision{
		{Action: Allow, Context: big},
		{Action: Allow, Context: "tail"},
	})
	if len(got.Context) > MaxContextBytes {
		t.Fatalf("context %d bytes exceeds cap %d", len(got.Context), MaxContextBytes)
	}
	if !strings.HasSuffix(got.Context, ContextTruncatedMarker) {
		t.Fatalf("missing truncation marker, suffix %q",
			got.Context[len(got.Context)-40:])
	}
	if !strings.HasPrefix(got.Context, "xxxx") {
		t.Fatalf("head of context must be preserved")
	}
}

func TestMerge_ContextAtCapNotTruncated(t *testing.T) {
	exact := strings.Repeat("y", MaxContextBytes)
	got := Merge([]Decision{{Action: Allow, Context: exact}})
	if got.Context != exact {
		t.Fatalf("context exactly at cap must pass through untouched")
	}
}

func TestMerge_ContextTruncationKeepsValidUTF8(t *testing.T) {
	// 3-byte runes behind 0..2 byte prefixes: at least one offset puts
	// the cut point mid-rune.
	for _, prefix := range []string{"", "a", "ab"} {
		big := prefix + strings.Repeat("€", MaxContextBytes)
		got := Merge([]Decision{{Action: Allow, Context: big}})
		if len(got.Context) > MaxContextBytes {
			t.Fatalf("prefix %q: context %d bytes exceeds cap", prefix, len(got.Context))
		}
		if !utf8.ValidString(got.Context) {
			t.Fatalf("prefix %q: truncated context is not valid UTF-8", prefix)
		}
		if !strings.HasSuffix(got.Context, ContextTruncatedMarker) {
			t.Fatalf("prefix %q: missing truncation marker", prefix)
		}
	}
}
