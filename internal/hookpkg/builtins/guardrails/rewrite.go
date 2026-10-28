package guardrails

import "strings"

// applyRewrites runs each strategy in order against cmd. Returns the
// rewritten command and ok=true when at least one strategy actually
// produced a change. ok=false signals the matcher to fall back to
// block (rewrite produced no effect → safer to deny than no-op
// allow).
func applyRewrites(cmd string, strategies []RewriteStrategy) (string, bool) {
	out := cmd
	changed := false
	for _, s := range strategies {
		var next string
		switch s.Strategy {
		case "strip_flag":
			next = stripFlag(out, s.Flag)
		case "replace_token":
			next = replaceToken(out, s.From, s.To)
		case "prepend_args":
			next = prependArgs(out, s.Args)
		default:
			next = out
		}
		if next != out {
			out = next
			changed = true
		}
	}
	return out, changed
}

// stripFlag removes flag from cmd. Treats flag as a whole token
// (delimited by whitespace, end-of-string, or shell separators) so
// "--no-verify" doesn't match a substring of "--no-verify-tag".
//
// Doesn't attempt to remove a flag's argument; v1 only targets
// boolean flags (--no-verify, --force). Flags with values land with
// proper shell-aware parsing later.
func stripFlag(cmd, flag string) string {
	if flag == "" {
		return cmd
	}
	// Try " <flag> " (mid-string) first — collapse one of the spaces.
	mid := " " + flag + " "
	if strings.Contains(cmd, mid) {
		return strings.Replace(cmd, mid, " ", 1)
	}
	// Then " <flag>" at end-of-string.
	tail := " " + flag
	if strings.HasSuffix(cmd, tail) {
		return strings.TrimSuffix(cmd, tail)
	}
	return cmd
}

// replaceToken substitutes from with to as whole-token replacements
// only (not substrings). Replaces all occurrences.
//
// Whole-token = surrounded on both sides by whitespace, shell
// separator (; | &), or string boundary.
func replaceToken(cmd, from, to string) string {
	if from == "" {
		return cmd
	}
	var b strings.Builder
	b.Grow(len(cmd))
	i := 0
	for i < len(cmd) {
		j := strings.Index(cmd[i:], from)
		if j < 0 {
			b.WriteString(cmd[i:])
			break
		}
		j += i
		end := j + len(from)
		leftOK := j == 0 || isTokenBoundary(cmd[j-1])
		rightOK := end == len(cmd) || isTokenBoundary(cmd[end])
		if leftOK && rightOK {
			b.WriteString(cmd[i:j])
			b.WriteString(to)
			i = end
		} else {
			// Not a whole-token match; emit chars up to and including j
			// and advance past it.
			b.WriteString(cmd[i : j+1])
			i = j + 1
		}
	}
	return b.String()
}

func isTokenBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', ';', '|', '&':
		return true
	}
	return false
}

// prependArgs inserts args after the program name (first whitespace-
// delimited token). Returns cmd unchanged if cmd is empty or args is
// empty.
//
// Example: prependArgs("git push origin x", ["--dry-run"]) →
//
//	"git --dry-run push origin x"
//
// This is intentionally simple: it does NOT try to skip leading
// `sudo`, `env`, `nice`, etc. — rules requiring such precision should
// use replace_token with a more specific from/to pair instead.
func prependArgs(cmd string, args []string) string {
	if cmd == "" || len(args) == 0 {
		return cmd
	}
	// Find the first space; insert args before it.
	for i := 0; i < len(cmd); i++ {
		if cmd[i] == ' ' || cmd[i] == '\t' {
			injected := strings.Join(args, " ")
			return cmd[:i] + " " + injected + cmd[i:]
		}
	}
	// No whitespace at all (single-token command); append.
	return cmd + " " + strings.Join(args, " ")
}
