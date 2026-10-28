package inbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/crtx"
	"hop.top/nerv/internal/ingest"
)

// sep separates digest fields (unit separator; cannot appear in ids).
const sep = "\x1f"

// MessageID derives the deterministic inbox message id for a foreign
// turn: the same logical turn observed via any ingest path (bus event,
// session-file scan, replay) yields the same id, making the store's
// existence check the dedup ledger. Same digest discipline as the
// producer's deterministic call ids.
func MessageID(envelopeID, turnID string) string {
	sum := sha256.Sum256([]byte(envelopeID + sep + turnID))
	return "m" + hex.EncodeToString(sum[:12])
}

// RouterConfig configures a Router. Every field is optional.
type RouterConfig struct {
	// Bus receives x.nerv.inbox_message.added lifecycle publishes on
	// first delivery. Nil disables publishing.
	Bus bus.Bus
	// LocalSessionsDir is this producer's own sessions directory,
	// used to correlate foreign lineage refs to local sessions.
	// Empty disables correlation.
	LocalSessionsDir string
	// AgentID is the recipient agent identity carried on lifecycle
	// events (the active profile when resolvable).
	AgentID string
}

// Router delivers accepted foreign turns (ingest.Message) into the
// inbox store: source attribution, lineage correlation, dedup on the
// deterministic message id, and an inbox_message.added lifecycle
// publish per first delivery. Failures are logged and swallowed —
// delivery is a side channel and must never crash ingestion.
type Router struct {
	store *Store
	cfg   RouterConfig

	delivered atomic.Uint64
	duplicate atomic.Uint64
	failed    atomic.Uint64
}

// NewRouter returns a Router delivering into store.
func NewRouter(store *Store, cfg RouterConfig) *Router {
	return &Router{store: store, cfg: cfg}
}

// Snapshot returns the router's delivery counters.
func (r *Router) Snapshot() map[string]uint64 {
	return map[string]uint64{
		"delivered": r.delivered.Load(),
		"duplicate": r.duplicate.Load(),
		"failed":    r.failed.Load(),
	}
}

// Handle is the ingest.Handler delivering one accepted message.
func (r *Router) Handle(m ingest.Message) {
	entry := Entry{
		SchemaVersion: EntrySchemaVersion,
		MsgID:         MessageID(m.EnvelopeID, m.TurnID),
		InboxID:       InboxForeignTurns,
		Kind:          KindForeignTurn,
		ReceivedAt:    m.ReceivedAt,
		Origin: EntryOrigin{
			Transport:     m.Origin.Transport,
			Topic:         m.Origin.Topic,
			BusSource:     m.Origin.BusSource,
			Path:          m.Origin.Path,
			SourceKind:    m.Origin.SourceKind,
			SourceVersion: m.Origin.SourceVersion,
		},
		EnvelopeID:      m.EnvelopeID,
		TurnID:          m.TurnID,
		Role:            m.Role,
		AgentID:         m.AgentID,
		InReplyToCallID: m.InReplyToCallID,
		Correlation:     r.correlate(m.Lineage),
		Content:         m.Content,
		Preview:         preview(m.Content),
	}
	if entry.ReceivedAt.IsZero() {
		entry.ReceivedAt = time.Now().UTC()
	}

	deliveredNew, err := r.store.Put(entry)
	if err != nil {
		r.failed.Add(1)
		log.Printf("inbox: deliver %s (fail-open): %v", entry.MsgID, err)
		return
	}
	if !deliveredNew {
		r.duplicate.Add(1)
		return
	}
	r.delivered.Add(1)
	r.publishAdded(entry)
}

// correlate resolves foreign lineage refs against the local sessions
// directory. Dispatch-nest linkage wins over fork linkage when both
// resolve (it names the call site that spawned the foreign session).
func (r *Router) correlate(l ingest.Lineage) *Correlation {
	dir := r.cfg.LocalSessionsDir
	if dir == "" {
		return nil
	}
	if id := l.DispatchedFromEnvelopeID; id != "" && localSessionExists(dir, id) {
		return &Correlation{LocalSessionID: id, Relation: "dispatched_from"}
	}
	if id := l.ParentID; id != "" && localSessionExists(dir, id) {
		return &Correlation{LocalSessionID: id, Relation: "parent"}
	}
	return nil
}

// localSessionExists probes for <dir>/<id>.jsonl, rejecting ids that
// could escape the directory.
func localSessionExists(dir, id string) bool {
	if err := safeSegment(id); err != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, id+".jsonl"))
	return err == nil
}

// addedPayload is the x.nerv.inbox_message.added payload, shaped per
// the spec/events.yaml catalog entry.
type addedPayload struct {
	SchemaVersion   string `json:"schema_version"`
	MsgID           string `json:"msg_id"`
	AgentID         string `json:"agent_id,omitempty"`
	InboxID         string `json:"inbox_id"`
	SourceSessionID string `json:"source_session_id"`
	Kind            string `json:"kind"`
	TS              string `json:"ts"`
	PayloadRef      string `json:"payload_ref"`
}

// publishAdded emits the lifecycle event for a first delivery.
func (r *Router) publishAdded(e Entry) {
	if r.cfg.Bus == nil {
		return
	}
	ref, err := r.store.path(e.InboxID, e.MsgID)
	if err != nil {
		ref = ""
	}
	p := addedPayload{
		SchemaVersion:   "1.0",
		MsgID:           e.MsgID,
		AgentID:         r.cfg.AgentID,
		InboxID:         e.InboxID,
		SourceSessionID: e.EnvelopeID,
		Kind:            e.Kind,
		TS:              e.ReceivedAt.UTC().Format(time.RFC3339Nano),
		PayloadRef:      ref,
	}
	ev := bus.NewEvent(bus.Topic(crtx.TopicVendorInboxMessageAdded), crtx.BusSource, p)
	if err := r.cfg.Bus.Publish(context.Background(), ev); err != nil {
		log.Printf("inbox: publish %s (fail-open): %v", crtx.TopicVendorInboxMessageAdded, err)
	}
}

// previewMax caps the preview length in runes.
const previewMax = 120

// preview extracts the first text part's text, truncated.
func preview(content []json.RawMessage) string {
	for _, raw := range content {
		var part struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(raw, &part); err != nil {
			continue
		}
		if part.Type != "text" || part.Text == "" {
			continue
		}
		runes := []rune(part.Text)
		if len(runes) > previewMax {
			return string(runes[:previewMax-1]) + "…"
		}
		return part.Text
	}
	return ""
}
