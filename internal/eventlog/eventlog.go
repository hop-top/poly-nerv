// Package eventlog defines the event logging interface and types for
// nerv. The JSONL backend (stateless mode) is the v1 default; SQLite
// is deferred to the daemon build.
package eventlog

import (
	"time"

	"hop.top/nerv/internal/envelope"
)

// Logger writes and queries event log entries.
type Logger interface {
	// Write persists an envelope and its associated decision.
	Write(env *envelope.Envelope, decision interface{}) error

	// Query returns entries matching the filter criteria.
	Query(filter Filter) ([]Entry, error)
}

// Filter constrains a Query call.
type Filter struct {
	Event   string    // match envelope event name
	CLI     string    // match cli.name
	Session string    // match session.id
	Since   time.Time // entries at or after this time
	Limit   int       // max entries to return (0 = unlimited)
}

// Entry is one persisted log record.
type Entry struct {
	Envelope *envelope.Envelope `json:"envelope"`
	Decision interface{}        `json:"decision,omitempty"`
	Timings  map[string]int64   `json:"timings,omitempty"`
}
