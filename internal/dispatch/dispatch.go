// Package dispatch orchestrates the full nerv dispatch pipeline:
// event lookup → match filtering → handler execution → merge → log.
//
// The dispatcher is pure glue — it calls other packages but holds no
// business logic itself. Each phase is timed for observability.
package dispatch

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/crtx"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/eventlog"
	"hop.top/nerv/internal/merger"
)

// Dispatcher orchestrates the full dispatch pipeline.
//
// The cached config is held behind an atomic pointer so the daemon
// can swap it (SIGHUP reload) without blocking in-flight dispatches.
// Each Dispatch call snapshots the pointer once and uses that
// snapshot for the duration of the call; readers and writers don't
// share a mutex on the hot path.
type Dispatcher struct {
	config  atomic.Pointer[config.CachedConfig]
	logger  eventlog.Logger
	matcher *Matcher
	runner  *Runner
}

// New creates a Dispatcher from cached config, event logger, matcher, and runner.
func New(cfg *config.CachedConfig, logger eventlog.Logger, matcher *Matcher, runner *Runner) *Dispatcher {
	d := &Dispatcher{
		logger:  logger,
		matcher: matcher,
		runner:  runner,
	}
	d.config.Store(cfg)
	return d
}

// Reload atomically swaps the cached config in place. Safe to call
// from a signal handler goroutine; in-flight Dispatch calls keep the
// pre-reload snapshot for the rest of their execution.
func (d *Dispatcher) Reload(cfg *config.CachedConfig) {
	d.config.Store(cfg)
}

// Config returns the current cached config snapshot. Exposed for
// tests and observability — callers must not mutate the returned
// value.
func (d *Dispatcher) Config() *config.CachedConfig {
	return d.config.Load()
}

// Timings tracks per-phase timing for observability.
type Timings struct {
	ParseMs    int64 `json:"parse_ms"`
	MatchMs    int64 `json:"match_ms"`
	HandlersMs int64 `json:"handlers_ms"`
	MergeMs    int64 `json:"merge_ms"`
	LogMs      int64 `json:"log_ms"`
	TotalMs    int64 `json:"total_ms"`
}

// Result is the dispatch output.
type Result struct {
	Decision merger.Decision
	Timings  Timings
}

// Dispatch processes a single envelope through the full pipeline:
//
//  1. Match filtering (from CachedConfig.ByEvent + regex)
//  2. Disable filtering (per-hook flag + sidecar events + sidecar bindings)
//  3. Run handlers (exec for user hooks; in-process for built-ins)
//  4. Merge decisions
//  5. Log to event log
func (d *Dispatcher) Dispatch(env *envelope.Envelope) (*Result, error) {
	totalStart := time.Now()

	// Snapshot the cached config once so a mid-call reload doesn't
	// split the pipeline across two configs.
	cc := d.config.Load()

	// Phase 1: match filtering
	matchStart := time.Now()
	matched := d.matcher.Match(cc, env)
	matchElapsed := time.Since(matchStart)

	// Phase 2: disable filtering — split matched hooks into the ones
	// that will run versus the ones that get a synthesised skip
	// decision. Skip decisions are appended to the dispatch result and
	// flow through the event log so `nerv tail` + `nerv replay` see
	// them.
	live, skipped := splitDisabled(matched.Hooks, env, cc.Disabled)

	// Phase 3: run handlers
	handlersStart := time.Now()
	decisions := d.runner.RunHooks(live, env, cc.Config)
	handlersElapsed := time.Since(handlersStart)

	// Append synthesised skip decisions so the merger + event log
	// observe them. Skip decisions are Allow-equivalent w.r.t.
	// dispatch semantics: they never block, never warn, never
	// rewrite — they exist purely as audit signal.
	decisions = append(decisions, skipped...)

	// Phase 4: merge decisions
	mergeStart := time.Now()
	merged := merger.Merge(decisions)
	mergeElapsed := time.Since(mergeStart)

	timings := Timings{
		MatchMs:    matchElapsed.Milliseconds(),
		HandlersMs: handlersElapsed.Milliseconds(),
		MergeMs:    mergeElapsed.Milliseconds(),
	}

	// Phase 4: log
	logStart := time.Now()
	if d.logger != nil {
		if err := d.logger.Write(env, merged); err != nil {
			return nil, err
		}
	}
	logElapsed := time.Since(logStart)

	timings.LogMs = logElapsed.Milliseconds()

	// Phase 5: crtx export. Published after the log write so a
	// subscriber that re-reads persisted state never races the append
	// (events spec §4). Best-effort: publish failure never affects the
	// dispatch decision.
	d.publishCrtx(env)

	timings.TotalMs = time.Since(totalStart).Milliseconds()

	return &Result{
		Decision: merged,
		Timings:  timings,
	}, nil
}

// publishCrtx translates the envelope onto crtx bus emissions and
// publishes them on the runner's bus. No bus (stateless callers,
// replay) means no export; errors are logged and swallowed
// (fail-open per spec/failure-modes.md).
func (d *Dispatcher) publishCrtx(env *envelope.Envelope) {
	if d.runner == nil {
		return
	}
	b := d.runner.Bus()
	if b == nil {
		return
	}
	for _, em := range crtx.Translate(env) {
		e := bus.NewEvent(bus.Topic(em.Topic), crtx.BusSource, em.Payload)
		if err := b.Publish(context.Background(), e); err != nil {
			log.Printf("dispatch: crtx publish %s failed (fail-open): %v", em.Topic, err)
		}
	}
}
