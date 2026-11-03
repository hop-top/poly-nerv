package merger

// ToolInputKey is the top-level key of the canonical rewrite form.
const ToolInputKey = "tool_input"

// NormalizeRewrite returns the canonical form of a rewrite: an object
// with a top-level "tool_input" key. A rewrite that already has that key
// is canonical and returned as is; any other object is the bare tool
// input shorthand and is wrapped whole as {"tool_input": <object>}.
// Nil and non-object values are returned unchanged. Idempotent.
//
// A tool whose real input has a top-level key literally named
// "tool_input" must use the wrapped form.
func NormalizeRewrite(rw any) any {
	m, ok := rw.(map[string]any)
	if !ok {
		return rw
	}
	if _, has := m[ToolInputKey]; has {
		return m
	}
	return map[string]any{ToolInputKey: m}
}

// RewriteToolInput extracts the host tool input from a canonical
// rewrite. ok is false when rw is not an object or has no tool_input.
// The value is returned as is (adapters decide how to treat a
// non-object tool_input).
func RewriteToolInput(rw any) (any, bool) {
	m, ok := rw.(map[string]any)
	if !ok {
		return nil, false
	}
	ti, ok := m[ToolInputKey]
	return ti, ok
}

// composeRewrites folds rewrites left to right on canonical form:
// shallow merge of top-level payload keys, later keys replacing earlier
// ones whole (tool_input is replaced, not deep-merged). A non-object
// rewrite replaces everything accumulated so far.
func composeRewrites(rws []any) any {
	var acc any
	for _, rw := range rws {
		rw = NormalizeRewrite(rw)
		next, isObj := rw.(map[string]any)
		prev, prevObj := acc.(map[string]any)
		if !isObj || !prevObj {
			acc = rw
			continue
		}
		merged := make(map[string]any, len(prev)+len(next))
		for k, v := range prev {
			merged[k] = v
		}
		for k, v := range next {
			merged[k] = v
		}
		acc = merged
	}
	return acc
}
