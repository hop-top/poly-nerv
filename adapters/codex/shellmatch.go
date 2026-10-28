package codex

import (
	"regexp"

	"hop.top/nerv/internal/events"
)

// SyntheticEvent pairs a nerv event with synthesis metadata.
type SyntheticEvent struct {
	Event  events.Event
	Source string // source pattern name
}

// shellRule maps a regex to the synthetic event it produces.
type shellRule struct {
	name  string
	regex *regexp.Regexp
	event events.Event
}

// ShellMatcher inspects shell command strings from PreToolUse/PostToolUse
// and returns synthetic nerv events based on regex patterns.
type ShellMatcher struct {
	rules []shellRule
}

// NewShellMatcher creates a matcher with the default rule set.
func NewShellMatcher() *ShellMatcher {
	return &ShellMatcher{
		rules: defaultRules(),
	}
}

// Match returns all synthetic events triggered by the command string.
// Returns nil if no rules match.
func (m *ShellMatcher) Match(cmd string) []SyntheticEvent {
	var out []SyntheticEvent
	for _, r := range m.rules {
		if r.regex.MatchString(cmd) {
			out = append(out, SyntheticEvent{
				Event:  r.event,
				Source: r.name,
			})
		}
	}
	return out
}

// defaultRules returns the compiled shell regex dispatch rules.
func defaultRules() []shellRule {
	defs := []struct {
		name    string
		pattern string
		event   events.Event
	}{
		{"cd_command", `(?:^|&&\s*|;\s*)cd\s`, events.EventCwdChanged},
		{"worktree_add", `(?:^|&&\s*|;\s*)\bgit\b\s+worktree\s+add\b`, events.EventWorktreeCreate},
		{"worktree_remove", `(?:^|&&\s*|;\s*)\bgit\b\s+worktree\s+(?:remove|rm)\b`, events.EventWorktreeRemove},
		{"instructions_loaded", `(?:^|&&\s*|;\s*)\bcat\b\s+\S*CLAUDE\.md`, events.EventInstructionsLoaded},
		{"rm_rf_block", `(?:^|&&\s*|;\s*)\brm\b\s+-rf\b`, events.EventPermissionRequest},
	}

	rules := make([]shellRule, 0, len(defs))
	for _, d := range defs {
		rules = append(rules, shellRule{
			name:  d.name,
			regex: regexp.MustCompile(d.pattern),
			event: d.event,
		})
	}
	return rules
}
