package main

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/inbox"
	"hop.top/nerv/internal/ingest"
)

// inboxDir is the local delivery surface (consumer ADR state layout).
func inboxDir() string {
	return filepath.Join(nervStateDir(), "inbox")
}

// defaultScanRoots returns the well-known sibling-producer session
// directories that exist on this machine. Today that is the stem
// convention directory; configured ingest.scan_roots replace this
// default entirely.
func defaultScanRoots() []string {
	root := filepath.Join(filepath.Dir(nervStateDir()), "stem", "sessions")
	if info, err := os.Stat(root); err == nil && info.IsDir() {
		return []string{root}
	}
	return nil
}

// startIngest wires the crtx consume side onto the daemon bus: the
// bus consumer routes foreign turns into the inbox store, and — when
// any scan root is configured or discoverable — a scanner polls
// foreign session JSONL. Returns a stop function; a disabled config
// returns an inert no-op.
//
// Consumption is a daemon concern: stateless invocations never call
// this (consumer ADR §"Two ingest paths").
func startIngest(ic config.IngestConfig, b bus.Bus, agentID string) func() {
	if !ic.EnabledOrDefault() {
		return func() {}
	}

	store := inbox.NewStore(inboxDir())
	router := inbox.NewRouter(store, inbox.RouterConfig{
		Bus:              b,
		LocalSessionsDir: sessionsDir(),
		AgentID:          agentID,
	})
	acct := ingest.NewAccounts()
	unsub := ingest.NewConsumer(router.Handle, acct).Attach(b)

	roots := ic.ScanRoots
	if len(roots) == 0 {
		roots = defaultScanRoots()
	}

	ctx, cancel := context.WithCancel(context.Background())
	var done chan struct{}
	if len(roots) > 0 {
		scanner := ingest.NewScanner(ingest.ScannerConfig{
			Roots:       roots,
			ExcludeRoot: nervStateDir(),
			Interval:    time.Duration(ic.ScanIntervalSeconds) * time.Second,
		}, router.Handle, acct)
		done = make(chan struct{})
		go func() {
			defer close(done)
			scanner.Run(ctx)
		}()
	}

	return func() {
		cancel()
		unsub()
		if done != nil {
			<-done
		}
	}
}
