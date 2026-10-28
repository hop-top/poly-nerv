package crtx

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Metadata keys (reverse-DNS per envelope spec §2).
const (
	// MetaSynthesized marks envelopes auto-created from a turn event
	// because no envelope.created was observed (mid-session daemon
	// start, stateless invocation).
	MetaSynthesized = "io.hop-top.nerv.synthesized"
	// MetaSessionEnd carries the SessionEnd hook payload recorded when
	// the producing CLI session terminates (crtx v0.1 has no lifecycle
	// terminator topic).
	MetaSessionEnd = "io.hop-top.nerv.session_end"
)

// Envelope is the persisted crtx v0.1 conversation container. Content
// parts stay raw JSON: the recorder appends event payload fragments
// verbatim (events spec invariant) and never needs to walk them.
//
// The child-linkage fields (parent_id/fork_point, dispatched_from,
// injected_turns) are declared for spec completeness so spec-shaped
// envelopes round-trip losslessly; today's hook-driven producer path
// does not populate them.
type Envelope struct {
	CrtxVersion    string            `json:"crtx_version"`
	ID             string            `json:"id"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	Source         Source            `json:"source"`
	Turns          []Turn            `json:"turns"`
	ParentID       string            `json:"parent_id,omitempty"`
	ForkPoint      *int              `json:"fork_point,omitempty"`
	DispatchedFrom *DispatchedFrom   `json:"dispatched_from,omitempty"`
	InjectedTurns  []InjectedTurnRef `json:"injected_turns,omitempty"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
}

// DispatchedFrom names the open tool_call in a parent Envelope that
// spawned this one as a dispatch nest (envelope spec §7.2).
type DispatchedFrom struct {
	EnvelopeID string `json:"envelope_id"`
	CallID     string `json:"call_id"`
}

// InjectedTurnRef references a slice of Turns in another Envelope
// included as context (envelope spec §7.3).
type InjectedTurnRef struct {
	EnvelopeID          string `json:"envelope_id"`
	StartTurnID         string `json:"start_turn_id"`
	EndTurnID           string `json:"end_turn_id"`
	InjectedAfterTurnID string `json:"injected_after_turn_id,omitempty"`
}

// Turn is one contribution to the conversation (envelope spec §3).
type Turn struct {
	ID              string            `json:"id"`
	Role            string            `json:"role"`
	CreatedAt       time.Time         `json:"created_at"`
	Content         []json.RawMessage `json:"content"`
	AgentID         string            `json:"agent_id,omitempty"`
	InReplyToCallID string            `json:"in_reply_to_call_id,omitempty"`
	Metadata        map[string]any    `json:"metadata,omitempty"`
}

// SessionStore persists crtx envelopes following stem's JSONLStore
// convention: one file per session named <envelope-id>.jsonl holding
// exactly one line — the complete Envelope — atomically rewritten on
// every append. Consumers reading the directory need no knowledge of
// this producer.
//
// Concurrency: single-process safe; per-session mutexes serialize
// read-modify-write. Cross-process appends require external locking.
type SessionStore struct {
	dir   string
	locks sync.Map // map[string]*sync.Mutex
}

// NewSessionStore returns a store rooted at dir. The directory is
// created on first write.
func NewSessionStore(dir string) *SessionStore {
	return &SessionStore{dir: dir}
}

// errUnsafeID rejects session ids that could escape the store root.
var errUnsafeID = errors.New("crtx: unsafe session id")

func (s *SessionStore) path(id string) (string, error) {
	if id == "" || id == "." || id == ".." ||
		strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return "", fmt.Errorf("%w: %q", errUnsafeID, id)
	}
	return filepath.Join(s.dir, id+".jsonl"), nil
}

func (s *SessionStore) lock(id string) *sync.Mutex {
	if v, ok := s.locks.Load(id); ok {
		return v.(*sync.Mutex)
	}
	v, _ := s.locks.LoadOrStore(id, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// Create materializes the envelope file for id. Idempotent: an
// existing envelope (resumed session re-emitting created) is left
// untouched.
func (s *SessionStore) Create(id string, createdAt time.Time, src Source) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	mu := s.lock(id)
	mu.Lock()
	defer mu.Unlock()

	if _, err := os.Stat(p); err == nil {
		return nil // already exists; append-only semantics
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	env := newEnvelope(id, createdAt, src)
	return writeEnvelope(p, env)
}

// AppendTurn appends one Turn, auto-creating a synthesized skeleton
// when no envelope exists yet.
func (s *SessionStore) AppendTurn(id string, turn Turn) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	mu := s.lock(id)
	mu.Lock()
	defer mu.Unlock()

	env, err := s.loadOrSynthesize(p, id, turn.CreatedAt)
	if err != nil {
		return err
	}
	env.Turns = append(env.Turns, turn)
	env.UpdatedAt = time.Now().UTC()
	return writeEnvelope(p, env)
}

// SetMetadata records value under key in the envelope metadata and
// bumps updated_at. Auto-creates a synthesized skeleton when needed.
func (s *SessionStore) SetMetadata(id, key string, value any) error {
	p, err := s.path(id)
	if err != nil {
		return err
	}
	mu := s.lock(id)
	mu.Lock()
	defer mu.Unlock()

	env, err := s.loadOrSynthesize(p, id, time.Time{})
	if err != nil {
		return err
	}
	if env.Metadata == nil {
		env.Metadata = map[string]any{}
	}
	env.Metadata[key] = value
	env.UpdatedAt = time.Now().UTC()
	return writeEnvelope(p, env)
}

// loadOrSynthesize reads the envelope at p, or builds a skeleton for
// an unseen session. Caller holds the session lock.
func (s *SessionStore) loadOrSynthesize(p, id string, at time.Time) (*Envelope, error) {
	env, err := readEnvelope(p)
	if err == nil {
		return env, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	env = newEnvelope(id, at, Source{Kind: "custom", Version: "unknown"})
	env.Metadata = map[string]any{MetaSynthesized: true}
	return env, nil
}

func newEnvelope(id string, createdAt time.Time, src Source) *Envelope {
	return &Envelope{
		CrtxVersion: Version,
		ID:          id,
		CreatedAt:   createdAt.UTC(),
		UpdatedAt:   createdAt.UTC(),
		Source:      src,
		Turns:       []Turn{}, // schema: turns must be an array, never null
	}
}

func readEnvelope(path string) (*Envelope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	line := data
	if i := indexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return nil, fmt.Errorf("crtx: unmarshal envelope %s: %w", path, err)
	}
	if env.Turns == nil {
		env.Turns = []Turn{}
	}
	return &env, nil
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// writeEnvelope marshals env as a single JSONL line and atomically
// replaces the session file (temp + fsync + rename), so a concurrent
// reader never observes a torn envelope.
func writeEnvelope(path string, env *Envelope) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("crtx: mkdir: %w", err)
	}
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("crtx: marshal envelope: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return fmt.Errorf("crtx: open tmp: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("crtx: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("crtx: fsync: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("crtx: close: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("crtx: rename: %w", err)
	}

	// Persist the rename; best-effort (some filesystems error here
	// without anything actionable).
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
