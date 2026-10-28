// Package guardrails is the in-process built-in that evaluates
// PreToolUse envelopes against declarative rule packs and emits a
// block / rewrite / ask / escalate decision.
//
// See docs/architecture/guardrails-builtin.md for the design and
// spec/rulepacks/*.yaml for the v1 rule surface.
//
// Layered decision interface (extensible per ADR §"Decision interface"):
//
//	ToolUseDecision    — synchronous, no I/O      (allow / block / rewrite)
//	  └─ InteractiveDecision — adds sync user prompt (hitl)
//	       └─ Decision         — adds async escalation via bus inbox
//	                              (manager / peer-N)
//
// v1 slice ships ToolUseDecision only. InteractiveDecision and
// Decision arrive later.
package guardrails

import (
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// Outcome is a guardrail's verdict before it is converted to a
// merger.Decision. Kept separate from merger.Decision so the matcher
// can return rich metadata (matched rule id, rewrite preview, etc.)
// for diagnostics and testing without coupling to the dispatch
// surface.
type Outcome struct {
	// RuleID is the id of the rule that fired, or "" if no rule
	// matched.
	RuleID string

	// Action is allow / block / rewrite. hitl / manager / peer-N
	// resolve to one of these three before reaching dispatch.
	Action merger.Action

	// Reason is the human-readable explanation surfaced in deny /
	// rewrite-notice UX. Required for block and rewrite.
	Reason string

	// RewrittenCommand is set when Action == Rewrite. The dispatch
	// layer copies this back into envelope.tool_input.command.
	RewrittenCommand string

	// Destructive flags whether a backup must run before exec
	// (post-decision, pre-tool-call). Always paired with a non-empty
	// BackupStrategy.
	Destructive    bool
	BackupStrategy string
}

// ToolUseDecision is the synchronous evaluation interface. Pure
// function-shaped: same envelope + packs in → same outcome out.
//
// Implementations MUST NOT perform I/O. Backup, HITL prompts, and
// escalation happen at higher layers after this returns.
type ToolUseDecision interface {
	Evaluate(env *envelope.Envelope, packs []Pack, overrides map[string]string) Outcome
}

// InteractiveDecision extends ToolUseDecision with a synchronous user
// prompt. Used when a rule's mode is "hitl". Implementations block
// the daemon goroutine until the user replies.
//
// v1: not yet implemented. Stub interface declared here so
// the type hierarchy is visible in code review.
type InteractiveDecision interface {
	ToolUseDecision

	// AskUser returns true if the user approves the action.
	AskUser(rule Rule, env *envelope.Envelope) (approved bool, err error)
}

// Decision extends InteractiveDecision with async escalation via the
// kit/bus inbox. Used when a rule's mode is "manager" or "peer-N".
//
// v1: not yet implemented.
type Decision interface {
	InteractiveDecision

	// Escalate publishes an inbox_message_added event and returns a
	// reference the daemon polls until approval lands or timeout
	// expires.
	Escalate(rule Rule, env *envelope.Envelope, target string) (ref string, err error)

	// Resolve waits for the escalation to terminate. Returns the
	// final Outcome after approval, denial, or timeout.
	Resolve(ref string) (Outcome, error)
}
