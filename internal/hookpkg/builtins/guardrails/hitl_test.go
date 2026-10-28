package guardrails

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

func envHITL(cmd string) *envelope.Envelope {
	pl, _ := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": cmd},
	})
	return &envelope.Envelope{Event: "PreToolUse", Payload: pl}
}

// makeHITL constructs a HITLMatcher whose Prompt reads from input
// and writes to a captured buffer.
func makeHITL(input string) (*HITLMatcher, *bytes.Buffer) {
	out := &bytes.Buffer{}
	return &HITLMatcher{
		Inner:  NewMatcher(),
		Prompt: InteractivePrompt{In: strings.NewReader(input), Out: out},
	}, out
}

func TestHITL_ApprovedAllowsAction(t *testing.T) {
	packs, err := LoadPacks(rulepacksDir(t))
	if err != nil {
		t.Fatal(err)
	}
	h, out := makeHITL("y\n")
	// git-reset-hard defaults to hitl.
	o := h.Evaluate(envHITL("git reset --hard HEAD"), packs, nil)
	if o.Action != merger.Allow {
		t.Errorf("Action = %s, want Allow", o.Action)
	}
	if o.RuleID != "git-reset-hard" {
		t.Errorf("RuleID = %q, want git-reset-hard", o.RuleID)
	}
	if !strings.Contains(out.String(), "git-reset-hard") {
		t.Errorf("prompt did not mention rule id: %q", out.String())
	}
	if !strings.Contains(out.String(), "Proposed: git reset --hard HEAD") {
		t.Errorf("prompt did not echo proposed cmd: %q", out.String())
	}
}

func TestHITL_DeniedBlocksAction(t *testing.T) {
	packs, _ := LoadPacks(rulepacksDir(t))
	h, _ := makeHITL("n\n")
	o := h.Evaluate(envHITL("git reset --hard HEAD"), packs, nil)
	if o.Action != merger.Block {
		t.Errorf("Action = %s, want Block", o.Action)
	}
	if !strings.Contains(o.Reason, "user declined") {
		t.Errorf("reason = %q, want 'user declined'", o.Reason)
	}
}

func TestHITL_EmptyReplyTreatedAsDeny(t *testing.T) {
	packs, _ := LoadPacks(rulepacksDir(t))
	h, _ := makeHITL("\n")
	o := h.Evaluate(envHITL("git reset --hard HEAD"), packs, nil)
	if o.Action != merger.Block {
		t.Errorf("empty reply: Action = %s, want Block", o.Action)
	}
}

func TestHITL_NonHITLDelegatesToInner(t *testing.T) {
	packs, _ := LoadPacks(rulepacksDir(t))
	h, out := makeHITL("y\n") // input must be unread
	// git-no-verify defaults to block; HITL should NOT prompt.
	o := h.Evaluate(envHITL("git commit --no-verify -m x"), packs, nil)
	if o.Action != merger.Block {
		t.Errorf("Action = %s, want Block", o.Action)
	}
	if out.Len() != 0 {
		t.Errorf("non-hitl rule produced prompt output: %q", out.String())
	}
}

func TestHITL_DestructiveFlagsPropagate(t *testing.T) {
	packs, _ := LoadPacks(rulepacksDir(t))
	h, _ := makeHITL("y\n")
	o := h.Evaluate(envHITL("git reset --hard HEAD"), packs, nil)
	if !o.Destructive {
		t.Error("Destructive flag not propagated through HITL")
	}
	if o.BackupStrategy != "git_stash_full" {
		t.Errorf("BackupStrategy = %q, want git_stash_full", o.BackupStrategy)
	}
}
