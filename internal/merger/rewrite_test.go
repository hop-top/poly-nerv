package merger

import (
	"reflect"
	"testing"
)

func TestNormalizeRewrite(t *testing.T) {
	canon := map[string]any{"tool_input": map[string]any{"command": "ls"}}
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"bare wrapped", map[string]any{"command": "ls"}, canon},
		{"canonical kept", canon, canon},
		{"canonical extra key kept",
			map[string]any{"tool_input": map[string]any{"a": 1}, "prompt": "p"},
			map[string]any{"tool_input": map[string]any{"a": 1}, "prompt": "p"}},
		{"edge: real input key tool_input is canonical",
			map[string]any{"tool_input": "x"}, map[string]any{"tool_input": "x"}},
		{"edge: bare wrapped twice is not double wrapped",
			map[string]any{"tool_input": map[string]any{"command": "ls"}}, canon},
		{"nil", nil, nil},
		{"string", "ls", "ls"},
		{"array", []any{1}, []any{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeRewrite(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
			if !reflect.DeepEqual(NormalizeRewrite(got), got) {
				t.Fatalf("not idempotent: %#v", got)
			}
		})
	}
}

func TestRewriteToolInput(t *testing.T) {
	ti, ok := RewriteToolInput(map[string]any{"tool_input": map[string]any{"c": 1}})
	if !ok || !reflect.DeepEqual(ti, map[string]any{"c": 1}) {
		t.Fatalf("got %#v %v", ti, ok)
	}
	if _, ok := RewriteToolInput(map[string]any{"c": 1}); ok {
		t.Fatal("bare form has no tool_input")
	}
	if _, ok := RewriteToolInput("x"); ok {
		t.Fatal("non-object has no tool_input")
	}
}

func TestMerge_RewriteMixedForms(t *testing.T) {
	got := Merge([]Decision{
		{Action: Rewrite, Rewrite: map[string]any{"command": "ls"}},
		{Action: Warn, Message: "w"},
		{Action: Rewrite, Rewrite: map[string]any{"tool_input": map[string]any{"command": "ls -la"}}},
	})
	want := map[string]any{"tool_input": map[string]any{"command": "ls -la"}}
	if got.Action != Rewrite || !reflect.DeepEqual(got.Rewrite, want) || got.Message != "w" {
		t.Fatalf("got %#v", got)
	}
}

func TestMerge_RewriteShallowKeysAndWholeToolInput(t *testing.T) {
	got := Merge([]Decision{
		{Action: Rewrite, Rewrite: map[string]any{
			"tool_input": map[string]any{"command": "ls", "timeout": 5}, "prompt": "p1"}},
		{Action: Rewrite, Rewrite: map[string]any{
			"tool_input": map[string]any{"command": "pwd"}}},
	})
	want := map[string]any{
		"tool_input": map[string]any{"command": "pwd"}, // replaced whole, not deep merged
		"prompt":     "p1",                             // untouched key kept
	}
	if !reflect.DeepEqual(got.Rewrite, want) {
		t.Fatalf("got %#v, want %#v", got.Rewrite, want)
	}
}
