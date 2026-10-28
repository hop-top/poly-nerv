// Package ingest is the consume half of the crtx surface: it accepts
// foreign producers' crtx v0.1 events from the daemon bus and from
// foreign session JSONL files, enforces the trust/validation boundary,
// and hands accepted messages to a delivery handler.
//
// See docs/architecture/crtx-consumer.md for the full decision record:
// routing policy, self-origin filtering, and the drop-with-account
// contract (a malformed or hostile input is counted and logged, never
// a crash, never a partial delivery).
package ingest

import (
	"encoding/json"
	"sync/atomic"
	"time"
)

// Origin records where a message entered: the bus path knows topic and
// bus source; the scan path knows the file and the envelope's declared
// Source. Each path fills only what it can honestly attribute.
type Origin struct {
	// Transport is "bus" or "scan".
	Transport string `json:"transport"`
	// Topic is the bus topic the event arrived on (bus path).
	Topic string `json:"topic,omitempty"`
	// BusSource is the kit/bus Event.Source of the publisher (bus path).
	BusSource string `json:"bus_source,omitempty"`
	// Path is the session file the turn was read from (scan path).
	Path string `json:"path,omitempty"`
	// SourceKind / SourceVersion echo the foreign envelope's Source
	// (scan path; the bus path has no envelope in hand).
	SourceKind    string `json:"source_kind,omitempty"`
	SourceVersion string `json:"source_version,omitempty"`
}

// Lineage echoes the foreign envelope's linkage fields (envelope spec
// §7) so delivery can correlate to local sessions. Only the scan path
// can fill it — canonical turn events carry no envelope linkage.
type Lineage struct {
	// ParentID is the foreign envelope's parent_id (what-if fork).
	ParentID string
	// DispatchedFromEnvelopeID / DispatchedFromCallID mirror the
	// foreign envelope's dispatched_from linkage (dispatch nest).
	DispatchedFromEnvelopeID string
	DispatchedFromCallID     string
}

// Message is one accepted foreign turn, ready for delivery.
type Message struct {
	EnvelopeID      string
	TurnID          string
	Role            string
	AgentID         string
	InReplyToCallID string
	// Content carries the foreign ContentParts verbatim (unknown part
	// types preserved per envelope spec §6.6).
	Content    []json.RawMessage
	Origin     Origin
	Lineage    Lineage
	ReceivedAt time.Time
}

// Handler receives accepted messages. Implementations own dedup and
// persistence; ingest guarantees only that every Message passed the
// validation boundary.
type Handler func(Message)

// Accounts tracks the drop-with-account counters of the validation
// boundary. All counters are atomic; a single instance may be shared
// across the bus consumer and the scanner.
type Accounts struct {
	accepted  atomic.Uint64 // passed the boundary, handed to the handler
	self      atomic.Uint64 // own emissions (Event.Source == crtx.BusSource)
	invalid   atomic.Uint64 // failed schema validation / unparseable
	version   atomic.Uint64 // crtx_version outside the supported pin
	skipped   atomic.Uint64 // valid but not message-like (routing policy)
	extension atomic.Uint64 // x.<vendor>.* events (opaque, never routed)
	session   atomic.Uint64 // crtx.session.* lifecycle events
}

// NewAccounts returns a zeroed counter set.
func NewAccounts() *Accounts { return &Accounts{} }

// Snapshot returns a point-in-time copy of every counter.
func (a *Accounts) Snapshot() map[string]uint64 {
	return map[string]uint64{
		"accepted":  a.accepted.Load(),
		"self":      a.self.Load(),
		"invalid":   a.invalid.Load(),
		"version":   a.version.Load(),
		"skipped":   a.skipped.Load(),
		"extension": a.extension.Load(),
		"session":   a.session.Load(),
	}
}

// routableTurn is the routing policy of consumer-ADR §"What routing
// means": user / assistant / developer turns are messages; a system
// turn is a message only as a mid-flight interjection; tool turns are
// call mechanics.
func routableTurn(role, inReplyToCallID string) bool {
	switch role {
	case "user", "assistant", "developer":
		return true
	case "system":
		return inReplyToCallID != ""
	default:
		return false
	}
}
