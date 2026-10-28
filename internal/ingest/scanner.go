package ingest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hop.top/nerv/internal/crtx"
)

// defaultScanInterval paces the daemon poll loop when the config does
// not set one.
const defaultScanInterval = 5 * time.Second

// ScannerConfig configures a foreign-session scanner.
type ScannerConfig struct {
	// Roots are the directories to scan for foreign envelope JSONL
	// (one nested directory level deep, the established crtx reader
	// discipline).
	Roots []string
	// ExcludeRoot rejects any configured root inside it — the
	// producer's own state subtree must never feed the scanner
	// (self-origin rule, consumer ADR §"Self-origin filtering").
	ExcludeRoot string
	// Interval paces Run's poll loop; <= 0 selects the default (5s).
	Interval time.Duration
}

// fileStamp is the change-detection key: an unchanged (size, mtime)
// pair is not re-read.
type fileStamp struct {
	size    int64
	modTime time.Time
}

// Scanner polls foreign session directories and routes message-like
// turns from schema-valid crtx v0.1 envelopes. Re-reads after a file
// change re-deliver every routable turn; downstream dedup (message id
// = envelope+turn digest) makes the overlap a no-op.
//
// Scanner is not safe for concurrent ScanOnce calls; Run serializes
// them on one goroutine.
type Scanner struct {
	roots    []string
	interval time.Duration
	handler  Handler
	acct     *Accounts
	seen     map[string]fileStamp
}

// NewScanner builds a Scanner from cfg. Roots inside cfg.ExcludeRoot
// are dropped with a logged warning.
func NewScanner(cfg ScannerConfig, h Handler, acct *Accounts) *Scanner {
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultScanInterval
	}
	var roots []string
	for _, root := range cfg.Roots {
		if cfg.ExcludeRoot != "" && withinDir(cfg.ExcludeRoot, root) {
			log.Printf("ingest: scan root %s is inside own state dir %s; ignoring",
				root, cfg.ExcludeRoot)
			continue
		}
		roots = append(roots, root)
	}
	return &Scanner{
		roots:    roots,
		interval: interval,
		handler:  h,
		acct:     acct,
		seen:     make(map[string]fileStamp),
	}
}

// withinDir reports whether path is dir or inside dir.
func withinDir(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// Run polls every interval until ctx is done. An initial pass runs
// immediately.
func (s *Scanner) Run(ctx context.Context) {
	s.ScanOnce()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ScanOnce()
		}
	}
}

// ScanOnce walks every root once and processes changed session files.
func (s *Scanner) ScanOnce() {
	for _, root := range s.roots {
		for _, path := range sessionFiles(root) {
			s.scanFile(path)
		}
	}
}

// sessionFiles lists *.jsonl files one nested directory level deep.
func sessionFiles(root string) []string {
	top, err := os.ReadDir(root)
	if err != nil {
		return nil // missing / unreadable root: nothing to scan
	}
	var out []string
	for _, e := range top {
		full := filepath.Join(root, e.Name())
		if e.IsDir() {
			inner, err := os.ReadDir(full)
			if err != nil {
				continue
			}
			for _, ie := range inner {
				if ie.IsDir() || !strings.HasSuffix(ie.Name(), ".jsonl") {
					continue
				}
				out = append(out, filepath.Join(full, ie.Name()))
			}
			continue
		}
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		out = append(out, full)
	}
	return out
}

// scanFile applies the validation boundary to one session file and
// routes its message-like turns. Every failure is drop-with-account.
func (s *Scanner) scanFile(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	stamp := fileStamp{size: info.Size(), modTime: info.ModTime()}
	if prev, ok := s.seen[path]; ok && prev == stamp {
		return
	}
	// Record the stamp regardless of outcome: an invalid file is not
	// re-read until it changes again (a torn foreign write heals on
	// the writer's next rename).
	s.seen[path] = stamp

	line, err := lastLine(path)
	if err != nil {
		s.acct.invalid.Add(1)
		log.Printf("ingest: skip %s: %v", path, err)
		return
	}

	// Version gate before full validation (envelope spec §8: reject
	// unsupported majors — as a skip, not an error).
	var vgate struct {
		CrtxVersion string `json:"crtx_version"`
	}
	if err := json.Unmarshal(line, &vgate); err != nil {
		s.acct.invalid.Add(1)
		log.Printf("ingest: skip %s: not an envelope: %v", path, err)
		return
	}
	if vgate.CrtxVersion != crtx.Version {
		s.acct.version.Add(1)
		log.Printf("ingest: skip %s: crtx_version %q (supported: %q)",
			path, vgate.CrtxVersion, crtx.Version)
		return
	}

	if err := crtx.ValidateEnvelopeBytes(line); err != nil {
		s.acct.invalid.Add(1)
		log.Printf("ingest: skip %s: %v", path, err)
		return
	}

	var env crtx.Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		s.acct.invalid.Add(1)
		log.Printf("ingest: skip %s: decode: %v", path, err)
		return
	}

	var lineage Lineage
	lineage.ParentID = env.ParentID
	if env.DispatchedFrom != nil {
		lineage.DispatchedFromEnvelopeID = env.DispatchedFrom.EnvelopeID
		lineage.DispatchedFromCallID = env.DispatchedFrom.CallID
	}

	now := time.Now().UTC()
	for _, turn := range env.Turns {
		if !routableTurn(turn.Role, turn.InReplyToCallID) {
			s.acct.skipped.Add(1)
			continue
		}
		s.handler(Message{
			EnvelopeID:      env.ID,
			TurnID:          turn.ID,
			Role:            turn.Role,
			AgentID:         turn.AgentID,
			InReplyToCallID: turn.InReplyToCallID,
			Content:         turn.Content,
			Origin: Origin{
				Transport:     "scan",
				Path:          path,
				SourceKind:    env.Source.Kind,
				SourceVersion: env.Source.Version,
			},
			Lineage:    lineage,
			ReceivedAt: now,
		})
		s.acct.accepted.Add(1)
	}
}

// lastLine returns the last non-empty line of the file (the reader
// discipline: producers rewrite session files whole; the final line
// is the authoritative envelope).
func lastLine(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var last []byte
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		trimmed := bytes.TrimRight(line, "\r\n")
		if len(trimmed) > 0 {
			last = append(last[:0], trimmed...)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
	}
	if len(last) == 0 {
		return nil, errors.New("empty session file")
	}
	return last, nil
}
