package daemon

import (
	"testing"

	"hop.top/nerv/internal/merger"
)

func TestDecisionToProto_Context(t *testing.T) {
	p, err := decisionToProto(merger.Decision{Action: merger.Allow, Context: "ctx"})
	if err != nil {
		t.Fatal(err)
	}
	if p.GetContext() != "ctx" {
		t.Fatalf("context = %q, want ctx", p.GetContext())
	}
}
