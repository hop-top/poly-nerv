package merger

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func genAction() *rapid.Generator[Action] {
	return rapid.SampledFrom([]Action{Allow, Warn, Block, Rewrite})
}

func genDecision() *rapid.Generator[Decision] {
	return rapid.Custom(func(t *rapid.T) Decision {
		a := genAction().Draw(t, "action")
		return Decision{
			Action:  a,
			Message: rapid.String().Draw(t, "message"),
			Rewrite: map[string]string{
				"k": rapid.String().Draw(t, "rewrite-val"),
			},
		}
	})
}

func TestRapid_BlockAlwaysWins(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 20).Draw(t, "n")
		ds := make([]Decision, n)
		for i := range ds {
			ds[i] = genDecision().Draw(t, "decision")
		}
		// Inject a block at random position.
		pos := rapid.IntRange(0, n-1).Draw(t, "blockPos")
		ds[pos] = Decision{Action: Block, Message: "blocked"}

		got := Merge(ds)
		if got.Action != Block {
			t.Fatalf("block must win, got %s", got.Action)
		}
	})
}

func TestRapid_AllAllowsIsAllow(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 20).Draw(t, "n")
		ds := make([]Decision, n)
		for i := range ds {
			ds[i] = Decision{Action: Allow}
		}

		got := Merge(ds)
		if got.Action != Allow {
			t.Fatalf("all-allow must yield allow, got %s", got.Action)
		}
	})
}

func TestRapid_WarnMessagesPreserved(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 10).Draw(t, "n")
		ds := make([]Decision, n)
		var wantWarns []string
		for i := range ds {
			msg := rapid.StringN(1, 50, 50).Draw(t, "msg")
			ds[i] = Decision{Action: Warn, Message: msg}
			wantWarns = append(wantWarns, msg)
		}

		got := Merge(ds)
		for _, w := range wantWarns {
			if !strings.Contains(got.Message, w) {
				t.Fatalf("warn %q lost in merged message %q", w, got.Message)
			}
		}
	})
}
