package guardrails

import (
	"strings"
	"testing"

	"hop.top/nerv/internal/merger"
)

func TestIsAsyncMode(t *testing.T) {
	cases := map[string]bool{
		"manager": true,
		"peer-1":  true,
		"peer-3":  true,
		"peer-99": true,
		"peer-0":  false,
		"peer-":   false,
		"peer-x":  false,
		"hitl":    false,
		"block":   false,
		"allow":   false,
		"rewrite": false,
	}
	for mode, want := range cases {
		if got := isAsyncMode(mode); got != want {
			t.Errorf("isAsyncMode(%q) = %v, want %v", mode, got, want)
		}
	}
}

func TestEscalating_ManagerModeReturnsBlockWithReason(t *testing.T) {
	packs, _ := LoadPacks(rulepacksDir(t))
	e := NewEscalatingMatcher()
	overrides := map[string]string{"git-reset-hard": "manager"}
	o := e.Evaluate(envHITL("git reset --hard HEAD"), packs, overrides)
	if o.Action != merger.Block {
		t.Errorf("Action = %s, want Block (escalation stub)", o.Action)
	}
	if !strings.Contains(o.Reason, "manager escalation") {
		t.Errorf("reason missing 'manager escalation': %q", o.Reason)
	}
	if !strings.Contains(o.Reason, "blocking for safety") {
		t.Errorf("reason missing 'blocking for safety' fallback note: %q", o.Reason)
	}
}

func TestEscalating_PeerModeReturnsBlockWithReason(t *testing.T) {
	packs, _ := LoadPacks(rulepacksDir(t))
	e := NewEscalatingMatcher()
	overrides := map[string]string{"git-reset-hard": "peer-2"}
	o := e.Evaluate(envHITL("git reset --hard HEAD"), packs, overrides)
	if o.Action != merger.Block {
		t.Errorf("Action = %s, want Block", o.Action)
	}
	if !strings.Contains(o.Reason, "peer-2") {
		t.Errorf("reason missing 'peer-2': %q", o.Reason)
	}
}

func TestEscalating_NonAsyncDelegatesToHITL(t *testing.T) {
	packs, _ := LoadPacks(rulepacksDir(t))
	e := NewEscalatingMatcher()
	// Inject a deny-reply prompt so a hitl-mode rule resolves to Block
	// (we just want to verify escalate doesn't intercept).
	e.Inner.Prompt = InteractivePrompt{In: strings.NewReader("n\n"), Out: &noopWriter{}}
	o := e.Evaluate(envHITL("git reset --hard HEAD"), packs, nil)
	if o.Action != merger.Block {
		t.Errorf("Action = %s, want Block (hitl declined)", o.Action)
	}
	if !strings.Contains(o.Reason, "user declined") {
		t.Errorf("reason = %q, want 'user declined' from hitl path", o.Reason)
	}
}

func TestEscalating_AllowModeStillDelegates(t *testing.T) {
	packs, _ := LoadPacks(rulepacksDir(t))
	e := NewEscalatingMatcher()
	overrides := map[string]string{"git-reset-hard": "allow"}
	o := e.Evaluate(envHITL("git reset --hard HEAD"), packs, overrides)
	if o.Action != merger.Allow {
		t.Errorf("Action = %s, want Allow", o.Action)
	}
}

// noopWriter discards any writes — used so the HITL prompt's stderr
// output doesn't leak into test output.
type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }
