package eventlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"hop.top/kit/go/core/xdg"
	"hop.top/nerv/internal/envelope"
)

// JSONLLogger writes one JSON object per line to a date-rotated file
// under $XDG_STATE_HOME/nerv/events/. It is stateless — no open file
// handles are held between calls.
//
// The events/ subtree is the internal surface of the state root;
// crtx session envelopes live in the sibling sessions/ subtree that
// downstream indexers scan. The two must never share a namespace
// (docs/architecture/crtx-producer.md, state layout).
type JSONLLogger struct {
	// Dir overrides the log directory. When empty, defaults to the
	// events/ subtree of kit/xdg.StateDir("nerv") (honors
	// XDG_STATE_HOME, with OS-native fallbacks).
	Dir string
}

// Write appends a single JSON line to today's log file.
func (l *JSONLLogger) Write(
	env *envelope.Envelope,
	decision interface{},
) (err error) {
	entry := Entry{
		Envelope: env,
		Decision: decision,
		Timings:  map[string]int64{"write_ns": time.Now().UnixNano()},
	}

	dir := l.dir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("eventlog: mkdir: %w", err)
	}

	path := filepath.Join(dir, l.filename(env.Timestamp))
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o640)
	if err != nil {
		return fmt.Errorf("eventlog: open: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("eventlog: close: %w", cerr)
		}
	}()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("eventlog: marshal: %w", err)
	}
	data = append(data, '\n')

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("eventlog: write: %w", err)
	}
	return nil
}

// Query performs a linear scan of log files matching the filter.
func (l *JSONLLogger) Query(filter Filter) ([]Entry, error) {
	dir := l.dir()
	pattern := filepath.Join(dir, "events-*.jsonl")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("eventlog: glob: %w", err)
	}

	var results []Entry
	for _, path := range files {
		entries, err := l.scanFile(path, filter)
		if err != nil {
			return nil, err
		}
		results = append(results, entries...)
		if filter.Limit > 0 && len(results) >= filter.Limit {
			results = results[:filter.Limit]
			break
		}
	}
	return results, nil
}

func (l *JSONLLogger) scanFile(
	path string,
	filter Filter,
) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("eventlog: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }() // read-only

	var entries []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue // skip corrupt lines
		}
		if !l.matches(&e, filter) {
			continue
		}
		entries = append(entries, e)
		if filter.Limit > 0 && len(entries) >= filter.Limit {
			break
		}
	}
	return entries, sc.Err()
}

func (l *JSONLLogger) matches(e *Entry, f Filter) bool {
	if e.Envelope == nil {
		return false
	}
	if f.Event != "" && e.Envelope.Event != f.Event {
		return false
	}
	if f.CLI != "" && e.Envelope.CLI.Name != f.CLI {
		return false
	}
	if f.Session != "" && e.Envelope.Session.ID != f.Session {
		return false
	}
	if !f.Since.IsZero() && e.Envelope.Timestamp.Before(f.Since) {
		return false
	}
	return true
}

func (l *JSONLLogger) dir() string {
	if l.Dir != "" {
		return l.Dir
	}
	dir, err := xdg.StateDir("nerv")
	if err != nil {
		// Fail-open per nerv contract: a guard rejection or
		// path-resolution failure must not block the daemon.
		// Mirror the legacy fallback so existing deployments
		// keep writing logs to a sensible location.
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "state", "nerv", "events")
	}
	return filepath.Join(dir, "events")
}

func (l *JSONLLogger) filename(t time.Time) string {
	return fmt.Sprintf("events-%s.jsonl", t.Format("2006-01-02"))
}
