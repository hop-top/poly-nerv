// Package merger combines multiple hook decisions per the merge
// semantics defined in spec/handler-contract.md.
package merger

import "strings"

// MetaSkips is the metadata key whose value, when present, is a
// []map[string]any list of per-hook skip records. Merge concatenates
// these lists across decisions rather than overwriting, so a single
// merged decision carries the full audit trail when 2+ hooks skip on
// the same envelope. Mirrors dispatch.MetaSkips by string value; kept
// here to avoid an import cycle (merger must not depend on dispatch).
const MetaSkips = "skips"

// Action represents a handler decision.
type Action string

const (
	Allow   Action = "allow"
	Warn    Action = "warn"
	Block   Action = "block"
	Rewrite Action = "rewrite"
)

// Decision from a single handler.
type Decision struct {
	Action   Action
	Message  string
	Rewrite  any
	Metadata map[string]any
}

// Merge combines N decisions using spec merge semantics:
//
//	block > rewrite(compose) > warn(accumulate) > allow
//
// Rules:
//   - First block wins; remaining decisions ignored.
//   - Multiple rewrites compose left-to-right.
//   - Multiple warns concatenate (newline-separated).
//   - Error from a hook → treated as allow for that hook only.
func Merge(decisions []Decision) Decision {
	if len(decisions) == 0 {
		return Decision{Action: Allow}
	}

	var (
		warns    []string
		rewrites []any
		lastRew  Decision
		meta     = make(map[string]any)
	)

	for _, d := range decisions {
		// Accumulate metadata from every decision. The MetaSkips key
		// is special-cased: its list-of-records value is concatenated
		// across decisions so a multi-skip envelope preserves every
		// audit entry. All other keys take last-write-wins semantics
		// (caller responsibility to avoid collisions).
		for k, v := range d.Metadata {
			if k == MetaSkips {
				meta[k] = appendSkips(meta[k], v)
				continue
			}
			meta[k] = v
		}

		switch d.Action {
		case Block:
			out := Decision{
				Action:   Block,
				Message:  d.Message,
				Metadata: meta,
			}
			return out

		case Rewrite:
			rewrites = append(rewrites, d.Rewrite)
			lastRew = d

		case Warn:
			if d.Message != "" {
				warns = append(warns, d.Message)
			}

		case Allow:
			// nothing extra
		}
	}

	if len(meta) == 0 {
		meta = nil
	}

	// Rewrite wins over warn, but warn messages preserved.
	if len(rewrites) > 0 {
		return Decision{
			Action:   Rewrite,
			Message:  strings.Join(warns, "\n"),
			Rewrite:  lastRew.Rewrite,
			Metadata: meta,
		}
	}

	if len(warns) > 0 {
		return Decision{
			Action:   Warn,
			Message:  strings.Join(warns, "\n"),
			Metadata: meta,
		}
	}

	return Decision{
		Action:   Allow,
		Metadata: meta,
	}
}

// appendSkips concatenates two MetaSkips values into a single
// []map[string]any. Either side may be nil; non-list values are
// preserved by wrapping them (defensive — callers should always emit
// the canonical shape). The output is a freshly allocated slice so
// later mutations on either input don't bleed through.
func appendSkips(existing, incoming any) []map[string]any {
	lhs := toSkipList(existing)
	rhs := toSkipList(incoming)
	out := make([]map[string]any, 0, len(lhs)+len(rhs))
	out = append(out, lhs...)
	out = append(out, rhs...)
	return out
}

// toSkipList coerces a metadata value into the canonical skip-list
// shape. Returns nil for nil input; wraps a single map; returns
// []map[string]any inputs as-is.
func toSkipList(v any) []map[string]any {
	switch s := v.(type) {
	case nil:
		return nil
	case []map[string]any:
		return s
	case map[string]any:
		return []map[string]any{s}
	case []any:
		out := make([]map[string]any, 0, len(s))
		for _, item := range s {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}
