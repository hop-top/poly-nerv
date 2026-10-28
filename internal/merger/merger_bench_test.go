package merger

import "testing"

func decisions(n int, action Action) []Decision {
	ds := make([]Decision, n)
	for i := range n {
		ds[i] = Decision{
			Action:   action,
			Message:  "msg",
			Metadata: map[string]any{"k": i},
		}
	}
	return ds
}

func BenchmarkMerge_2Decisions(b *testing.B) {
	ds := []Decision{
		{Action: Allow, Message: "ok"},
		{Action: Warn, Message: "check"},
	}
	b.ResetTimer()
	for range b.N {
		_ = Merge(ds)
	}
}

func BenchmarkMerge_10Decisions(b *testing.B) {
	ds := decisions(10, Warn)
	b.ResetTimer()
	for range b.N {
		_ = Merge(ds)
	}
}

func BenchmarkMerge_AllAllow(b *testing.B) {
	ds := decisions(10, Allow)
	b.ResetTimer()
	for range b.N {
		_ = Merge(ds)
	}
}

func BenchmarkMerge_WithRewrite(b *testing.B) {
	ds := []Decision{
		{Action: Allow, Message: "ok"},
		{Action: Warn, Message: "caution"},
		{Action: Rewrite, Message: "rewrite1", Rewrite: map[string]string{"a": "b"}},
		{Action: Warn, Message: "caution2"},
		{Action: Rewrite, Message: "rewrite2", Rewrite: map[string]string{"c": "d"}},
	}
	b.ResetTimer()
	for range b.N {
		_ = Merge(ds)
	}
}
