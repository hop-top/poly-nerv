package guardrails

import (
	"fmt"
	"strconv"
	"strings"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// EscalatingMatcher is the v1 placeholder for the full Decision
// interface. It wraps HITLMatcher and intercepts manager / peer-N
// modes; both currently resolve to Block with an explanatory reason
// because the bus reference required to publish an
// x.nerv.inbox_message.added event (crtx.TopicVendorInboxMessageAdded)
// is not yet plumbed through BuiltinRuntimeCfg.
//
// When that wiring lands, this file becomes the real implementation
// site: Escalate publishes the event, Resolve polls the inbox until
// approval / denial / timeout, and the synchronous Outcome flips to
// Allow on quorum.
//
// Until then, the layered design is realized — callers can compose
// EscalatingMatcher → HITLMatcher → Matcher and the manager / peer-N
// modes degrade to Block with a clear reason instead of leaking
// through as silent allows.
type EscalatingMatcher struct {
	Inner *HITLMatcher
}

// NewEscalatingMatcher wraps a default HITLMatcher.
func NewEscalatingMatcher() *EscalatingMatcher {
	return &EscalatingMatcher{Inner: NewHITLMatcher()}
}

// Evaluate dispatches manager / peer-N modes to Escalate; everything
// else delegates to the inner HITLMatcher.
func (e *EscalatingMatcher) Evaluate(env *envelope.Envelope, packs []Pack, overrides map[string]string) Outcome {
	rule, mode, ok := findFirstMatch(env, packs, overrides)
	if !ok {
		return Outcome{Action: merger.Allow}
	}
	if !isAsyncMode(mode) {
		return e.Inner.Evaluate(env, packs, overrides)
	}

	target := strings.TrimPrefix(mode, "peer-")
	if target == mode {
		target = "manager"
	}
	_, err := e.Escalate(*rule, env, target)
	return Outcome{
		RuleID:         rule.ID,
		Action:         merger.Block,
		Reason:         escalationStubReason(rule.ID, mode, err),
		Destructive:    rule.Destructive,
		BackupStrategy: rule.BackupStrategy,
	}
}

// AskUser delegates to the inner HITLMatcher (InteractiveDecision
// contract).
func (e *EscalatingMatcher) AskUser(rule Rule, env *envelope.Envelope) (bool, error) {
	return e.Inner.AskUser(rule, env)
}

// Escalate is the bus-publish step. Currently a no-op stub that
// returns "" + nil error. Real implementation requires:
//
//   - Bus instance threaded into BuiltinRuntimeCfg
//   - aps lookup of <target> profile to resolve recipient agent_id
//   - Construction of an inbox_message_added payload per
//     spec/events.yaml shape (msg_id, kind=approval-request,
//     correlation key)
//   - Publish via bus.Publish(ctx,
//     crtx.TopicVendorInboxMessageAdded, payload) — topics follow the
//     four-segment x.nerv.inbox_message.<action> convention; the bare
//     snake_case event name is never a bus topic
//   - Subscribe + filter by msg_id for the matching
//     x.nerv.inbox_message.promoted / x.nerv.inbox_message.dropped
//     reply
//
// See ADR §"Decision interface chain" and US-0023.
func (e *EscalatingMatcher) Escalate(rule Rule, env *envelope.Envelope, target string) (string, error) {
	_ = rule
	_ = env
	_ = target
	return "", nil
}

// Resolve waits for an escalation to terminate. Stub: returns Block
// since Escalate did not actually publish anything.
func (e *EscalatingMatcher) Resolve(ref string) (Outcome, error) {
	_ = ref
	return Outcome{
		Action: merger.Block,
		Reason: "guardrails: async escalation not yet wired",
	}, nil
}

// isAsyncMode returns true for "manager" or "peer-N" with N > 0.
func isAsyncMode(mode string) bool {
	if mode == "manager" {
		return true
	}
	if !strings.HasPrefix(mode, "peer-") {
		return false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(mode, "peer-"))
	return err == nil && n > 0
}

func escalationStubReason(ruleID, mode string, err error) string {
	if err != nil {
		return fmt.Sprintf(
			"guardrails(%s): %s escalation failed (%v); blocking for safety. "+
				"Async escalation requires bus wiring.",
			ruleID, mode, err)
	}
	return fmt.Sprintf(
		"guardrails(%s): %s escalation queued but resolution not yet wired; "+
			"blocking for safety. See ADR §\"Decision interface chain\".",
		ruleID, mode)
}
