package guardrails

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// InteractivePrompt is the dependency the HITL implementation uses
// for I/O. Production wires os.Stdin / os.Stderr; tests inject
// strings.Reader and bytes.Buffer.
type InteractivePrompt struct {
	In  io.Reader
	Out io.Writer
}

// DefaultPrompt returns an InteractivePrompt wired to the daemon's
// standard streams.
func DefaultPrompt() InteractivePrompt {
	return InteractivePrompt{In: os.Stdin, Out: os.Stderr}
}

// HITLMatcher implements InteractiveDecision by wrapping a Matcher
// and adding a stderr prompt for hitl-mode rules.
type HITLMatcher struct {
	Inner  *Matcher
	Prompt InteractivePrompt
}

// NewHITLMatcher constructs a HITLMatcher with the default os
// streams. Tests should construct with custom Prompt.
func NewHITLMatcher() *HITLMatcher {
	return &HITLMatcher{Inner: NewMatcher(), Prompt: DefaultPrompt()}
}

// Evaluate first runs the underlying matcher; if the chosen mode
// was hitl (which the inner matcher would have collapsed to Block),
// HITLMatcher checks the original rule's mode — if it WAS hitl,
// run the prompt and convert the user's reply into the final
// outcome.
//
// To detect hitl, we re-walk the packs ourselves rather than try to
// thread the original mode through Outcome. Cheap; same regex
// engine.
func (h *HITLMatcher) Evaluate(env *envelope.Envelope, packs []Pack, overrides map[string]string) Outcome {
	rule, mode, ok := findFirstMatch(env, packs, overrides)
	if !ok {
		return Outcome{Action: merger.Allow}
	}

	if mode != "hitl" {
		// Non-HITL: defer to the synchronous matcher (which handles
		// allow / block / rewrite + the higher-mode block fallback).
		return h.Inner.Evaluate(env, packs, overrides)
	}

	approved, err := h.AskUser(*rule, env)
	if err != nil || !approved {
		return Outcome{
			RuleID:         rule.ID,
			Action:         merger.Block,
			Reason:         hitlDenyReason(rule, err),
			Destructive:    rule.Destructive,
			BackupStrategy: rule.BackupStrategy,
		}
	}
	return Outcome{
		RuleID:         rule.ID,
		Action:         merger.Allow,
		Reason:         "guardrails: user approved (hitl)",
		Destructive:    rule.Destructive,
		BackupStrategy: rule.BackupStrategy,
	}
}

// AskUser writes the rule reason + proposed command to Prompt.Out
// and reads a single line from Prompt.In. Returns true when the
// reply is "y" or "Y" (with optional trailing newline). Any other
// reply, EOF, or read error returns false.
func (h *HITLMatcher) AskUser(rule Rule, env *envelope.Envelope) (bool, error) {
	cmd := commandFromOutcomeContext(env)

	_, _ = fmt.Fprintf(h.Prompt.Out, "\n──── guardrails: %s ────\n", rule.ID)
	_, _ = fmt.Fprintf(h.Prompt.Out, "%s\n", strings.TrimSpace(rule.Reason))
	_, _ = fmt.Fprintf(h.Prompt.Out, "Proposed: %s\n", cmd)
	_, _ = fmt.Fprintf(h.Prompt.Out, "Allow? [y/N]: ")

	reader := bufio.NewReader(h.Prompt.In)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes", nil
}

func hitlDenyReason(rule *Rule, err error) string {
	if err != nil {
		return fmt.Sprintf("guardrails(%s): hitl read error: %v; denying for safety", rule.ID, err)
	}
	return fmt.Sprintf("guardrails(%s): user declined", rule.ID)
}

// findFirstMatch walks the packs in load order, returning the first
// rule whose pattern matches and the resolved mode (after applying
// the user override map). Returns ok=false if no rule applies.
func findFirstMatch(env *envelope.Envelope, packs []Pack, overrides map[string]string) (*Rule, string, bool) {
	pl, ok := parsePayload(env)
	if !ok {
		return nil, "", false
	}
	cmd, _ := pl.ToolInput["command"].(string)
	if cmd == "" {
		return nil, "", false
	}
	for _, p := range packs {
		if p.AppliesTo.Tool != pl.ToolName {
			continue
		}
		for i := range p.Rules {
			r := &p.Rules[i]
			if !r.Regex().MatchString(cmd) {
				continue
			}
			mode := r.DefaultMode
			if v, ok := overrides[r.ID]; ok && v != "" {
				mode = v
			}
			return r, mode, true
		}
	}
	return nil, "", false
}
