package guardrails

import (
	"encoding/json"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// preToolUsePayload is the parsed shape of a PreToolUse envelope's
// payload, per spec/events.yaml. Only the fields the matcher needs.
type preToolUsePayload struct {
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
}

// Matcher is the v1 ToolUseDecision implementation. Stateless;
// holds no fields beyond the loaded packs (passed per-call so tests
// can inject custom packs without touching the filesystem).
type Matcher struct{}

// NewMatcher constructs a default Matcher.
func NewMatcher() *Matcher { return &Matcher{} }

// Evaluate walks the packs in load order, returning the first rule
// whose pattern matches the envelope's command. Returns Outcome with
// RuleID="" + Action=Allow when no rule matches.
//
// overrides is the user's per-rule mode override map (from
// hooks.yaml → builtin.guardrails.rules). Empty / nil means use each
// rule's default_mode.
func (m *Matcher) Evaluate(env *envelope.Envelope, packs []Pack, overrides map[string]string) Outcome {
	pl, ok := parsePayload(env)
	if !ok {
		return Outcome{Action: merger.Allow}
	}
	cmd, _ := pl.ToolInput["command"].(string)
	if cmd == "" {
		return Outcome{Action: merger.Allow}
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
			return outcomeFor(r, cmd, mode)
		}
	}

	return Outcome{Action: merger.Allow}
}

// parsePayload decodes the envelope's Payload as a PreToolUse payload.
// Returns (zero, false) for non-PreToolUse events or malformed JSON;
// the caller treats both as "no rule applies → allow".
func parsePayload(env *envelope.Envelope) (preToolUsePayload, bool) {
	if env == nil || env.Event != "PreToolUse" || len(env.Payload) == 0 {
		return preToolUsePayload{}, false
	}
	var pl preToolUsePayload
	if err := json.Unmarshal(env.Payload, &pl); err != nil {
		return preToolUsePayload{}, false
	}
	return pl, true
}

// outcomeFor maps a (rule, mode) pair to an Outcome. Modes higher
// than ToolUseDecision can produce (hitl / manager / peer-N) fall
// back to Block in v1; later releases fill in the higher
// layers.
func outcomeFor(r *Rule, cmd, mode string) Outcome {
	out := Outcome{
		RuleID:         r.ID,
		Reason:         r.Reason,
		Destructive:    r.Destructive,
		BackupStrategy: r.BackupStrategy,
	}
	switch mode {
	case "allow":
		out.Action = merger.Allow
	case "block":
		out.Action = merger.Block
	case "rewrite":
		rewritten, ok := applyRewrites(cmd, r.Rewrites)
		if !ok {
			out.Action = merger.Block
			out.Reason = "guardrails: rewrite strategy produced no change; falling back to block"
			return out
		}
		out.Action = merger.Rewrite
		out.RewrittenCommand = rewritten
	default:
		// hitl / manager / peer-N → block in v1 with reason hinting
		// at the deferred mode. Higher layers replace this.
		out.Action = merger.Block
	}
	return out
}
