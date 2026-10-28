// Package dispatch — disable enforcement.
//
// Three filter modes (any one short-circuits the handler exec):
//
//  1. Per-hook flag    → HookEntry.Disabled == true
//  2. Per-event list   → .nerv/disabled.yaml events: includes env.Event
//  3. Per-binding list → .nerv/disabled.yaml bindings: includes (hook, event)
//
// On match the runner is skipped for that hook; instead a synthesised
// Allow decision is emitted carrying a one-entry metadata.skips list so
// downstream consumers (event log, replay, tail) can recognise the skip
// and the merger can concatenate multiple skips onto a single envelope
// without overwriting earlier entries.
package dispatch

import (
	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// Skip metadata keys. Stable strings so external tooling can match
// without reaching into private constants.
//
// MetaSkips is the top-level metadata key on a merged decision; its
// value is []map[string]any where each map carries SkipHook, SkipEvent,
// and SkipReason for one skipped hook. The merger concatenates these
// lists across decisions on the same envelope, so a decision arising
// from N disabled hooks ends up with len(skips)==N.
const (
	MetaSkips = "skips"

	SkipHook   = "hook"
	SkipEvent  = "event"
	SkipReason = "reason"
)

// Skip reasons emitted on disabled-* matches.
const (
	ReasonDisabledHook    = "disabled-hook"
	ReasonDisabledEvent   = "disabled-event"
	ReasonDisabledBinding = "disabled-binding"
)

// splitDisabled partitions matched into the hooks that will execute
// and a parallel slice of synthesised skip decisions for those that
// were filtered. Order in live preserves the source order; skip
// decisions appear in the same order as the disabled hooks they
// describe so the merged metadata is reproducible.
//
// A nil sidecar is treated as "nothing event/binding-disabled"; the
// per-hook flag is still honoured.
func splitDisabled(
	matched []config.HookEntry,
	env *envelope.Envelope,
	sidecar *config.Disabled,
) (live []config.HookEntry, skipped []merger.Decision) {
	if len(matched) == 0 {
		return nil, nil
	}
	live = make([]config.HookEntry, 0, len(matched))
	for _, h := range matched {
		switch {
		case h.Disabled:
			skipped = append(skipped, skipDecision(h.Name, env.Event, ReasonDisabledHook))
		case sidecar.HasEvent(env.Event):
			skipped = append(skipped, skipDecision(h.Name, env.Event, ReasonDisabledEvent))
		case sidecar.HasBinding(h.Name, env.Event):
			skipped = append(skipped, skipDecision(h.Name, env.Event, ReasonDisabledBinding))
		default:
			live = append(live, h)
		}
	}
	return live, skipped
}

// skipDecision builds an Allow decision annotated as a skip so the
// merger remains a no-op for skipped hooks while the event log still
// records the audit trail. Each skip carries a single-entry skips list;
// the merger concatenates lists across decisions, preserving the full
// audit even when 2+ hooks skip on the same envelope.
func skipDecision(hook, event, reason string) merger.Decision {
	return merger.Decision{
		Action:  merger.Allow,
		Message: "skipped: " + reason,
		Metadata: map[string]any{
			MetaSkips: []map[string]any{
				{
					SkipHook:   hook,
					SkipEvent:  event,
					SkipReason: reason,
				},
			},
		},
	}
}
