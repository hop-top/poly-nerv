// Package inbox is the local delivery surface for routed messages:
// one JSON document per message under
// <state>/nerv/inbox/<inbox-id>/<msg-id>.json, written atomically.
//
// The container/item split mirrors the inbox_message_* event catalog
// (inbox_id + msg_id), so later flows (guardrails escalation,
// open-questions triage) can adopt sibling containers without
// migration. See docs/architecture/crtx-consumer.md §"The inbox".
package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// EntrySchemaVersion is the inbox entry document version.
const EntrySchemaVersion = "1.0"

// InboxForeignTurns is the container routed foreign turns land in.
const InboxForeignTurns = "foreign-turns"

// KindForeignTurn is the entry kind for routed foreign turns.
const KindForeignTurn = "foreign-turn"

// EntryOrigin records where the message entered (mirrors
// ingest.Origin; declared locally so the store stays dependency-free).
type EntryOrigin struct {
	Transport     string `json:"transport"`
	Topic         string `json:"topic,omitempty"`
	BusSource     string `json:"bus_source,omitempty"`
	Path          string `json:"path,omitempty"`
	SourceKind    string `json:"source_kind,omitempty"`
	SourceVersion string `json:"source_version,omitempty"`
}

// Correlation links an entry to a local session via the foreign
// envelope's lineage refs.
type Correlation struct {
	LocalSessionID string `json:"local_session_id"`
	// Relation names the linkage that matched: "parent" (what-if
	// fork) or "dispatched_from" (dispatch nest).
	Relation string `json:"relation"`
}

// Entry is one delivered inbox message.
type Entry struct {
	SchemaVersion   string            `json:"schema_version"`
	MsgID           string            `json:"msg_id"`
	InboxID         string            `json:"inbox_id"`
	Kind            string            `json:"kind"`
	ReceivedAt      time.Time         `json:"received_at"`
	Origin          EntryOrigin       `json:"origin"`
	EnvelopeID      string            `json:"envelope_id"`
	TurnID          string            `json:"turn_id"`
	Role            string            `json:"role"`
	AgentID         string            `json:"agent_id,omitempty"`
	InReplyToCallID string            `json:"in_reply_to_call_id,omitempty"`
	Correlation     *Correlation      `json:"correlation,omitempty"`
	Content         []json.RawMessage `json:"content"`
	Preview         string            `json:"preview,omitempty"`
}

// Store persists inbox entries. Concurrency: single-process safe;
// per-message mutexes serialize the exists-check + write pair.
type Store struct {
	dir   string
	locks sync.Map // map[string]*sync.Mutex
}

// NewStore returns a store rooted at dir. Directories are created on
// first write.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// errUnsafeID rejects ids that could escape the store root.
var errUnsafeID = errors.New("inbox: unsafe id")

func safeSegment(id string) error {
	if id == "" || id == "." || id == ".." ||
		strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return fmt.Errorf("%w: %q", errUnsafeID, id)
	}
	return nil
}

func (s *Store) path(inboxID, msgID string) (string, error) {
	if err := safeSegment(inboxID); err != nil {
		return "", err
	}
	if err := safeSegment(msgID); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, inboxID, msgID+".json"), nil
}

func (s *Store) lock(key string) *sync.Mutex {
	if v, ok := s.locks.Load(key); ok {
		return v.(*sync.Mutex)
	}
	v, _ := s.locks.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// Put delivers an entry. Returns false when an entry with the same
// (inbox_id, msg_id) already exists — the existing document is left
// untouched (the store is the dedup ledger; first delivery wins).
func (s *Store) Put(e Entry) (bool, error) {
	p, err := s.path(e.InboxID, e.MsgID)
	if err != nil {
		return false, err
	}
	mu := s.lock(e.InboxID + "/" + e.MsgID)
	mu.Lock()
	defer mu.Unlock()

	if _, err := os.Stat(p); err == nil {
		return false, nil // duplicate: no-op
	}

	data, err := json.Marshal(e)
	if err != nil {
		return false, fmt.Errorf("inbox: marshal entry: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return false, fmt.Errorf("inbox: mkdir: %w", err)
	}
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return false, fmt.Errorf("inbox: open tmp: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return false, fmt.Errorf("inbox: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return false, fmt.Errorf("inbox: fsync: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("inbox: close: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("inbox: rename: %w", err)
	}
	return true, nil
}

// List returns every entry across all containers, newest first.
// Unparseable files are skipped (the store never fails a read because
// one document is corrupt).
func (s *Store) List() ([]Entry, error) {
	containers, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Entry
	for _, c := range containers {
		if !c.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(s.dir, c.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			e, err := readEntry(filepath.Join(s.dir, c.Name(), f.Name()))
			if err != nil {
				continue
			}
			out = append(out, *e)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ReceivedAt.After(out[j].ReceivedAt)
	})
	return out, nil
}

// ErrNotFound reports a missing entry.
var ErrNotFound = errors.New("inbox: entry not found")

// Get returns the entry with msgID, searching every container.
func (s *Store) Get(msgID string) (*Entry, error) {
	if err := safeSegment(msgID); err != nil {
		return nil, err
	}
	containers, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	for _, c := range containers {
		if !c.IsDir() {
			continue
		}
		p := filepath.Join(s.dir, c.Name(), msgID+".json")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		return readEntry(p)
	}
	return nil, ErrNotFound
}

func readEntry(path string) (*Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("inbox: unmarshal %s: %w", path, err)
	}
	if e.MsgID == "" {
		return nil, fmt.Errorf("inbox: %s: missing msg_id", path)
	}
	return &e, nil
}
