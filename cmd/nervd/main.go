// Package main is nervd — the minimal hot-path hook dispatcher.
//
// Build with CGO_ENABLED=0 for a static binary:
//
//	CGO_ENABLED=0 go build -o bin/nervd ./cmd/nervd
//
// Modes:
//
//   - Stateless (default): reads envelope JSON from stdin,
//     evaluates against the hook pipeline, writes decision
//     JSON to stdout, then exits.
//
//   - Daemon (NERV_SOCKET set): listens on a Unix domain socket
//     for envelope events from CLI adapters. Serves the
//     nerv.v1.EventService over h2c via internal/daemon. Long-running.
//
// No TUI, no upgrade, no marketplace — keep this binary tiny and fast.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/adapters/claude"
	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/daemon"
	"hop.top/nerv/internal/dispatch"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/eventlog"
	"hop.top/nerv/internal/hookpkg/builtins"
	"hop.top/nerv/internal/merger"

	// Built-in registrations: each subpackage's init() appends to
	// internal/hookpkg/builtins.All. Dispatch reads that slice.
	_ "hop.top/nerv/internal/hookpkg/builtins/guardrails"

	_ "hop.top/kit/go/console/cli"
	_ "hop.top/kit/go/console/log"
	_ "hop.top/kit/go/core/config"
	_ "hop.top/kit/go/core/xdg"
)

// daemonBus is the process-wide event bus, built from the hooks.yaml
// bus section (adapter + sinks; see cmd/nervd/bus.go). Defaults to the
// in-memory adapter: in-process, synchronous-for-sync-handlers, async
// via goroutines for async handlers, no persistence. Exposed via
// Bus() so dispatch and other in-process subsystems can publish.
var daemonBus bus.Bus

// Bus returns the daemon-scoped bus instance, or nil if uninitialized
// (e.g. during early boot or in tests that bypass main).
func Bus() bus.Bus { return daemonBus }

// initBus builds the daemon-scoped bus from the loaded bus config and
// registers it as the process-wide instance. Returns the constructed
// bus so main() can defer Close on it.
func initBus(bc config.BusConfig) bus.Bus {
	b := buildBus(bc)
	daemonBus = b
	return b
}

// initActiveProfile is a startup hook: resolves the aps active
// profile and emits a one-line warning to stderr when unresolved.
// Returns the resolved id (empty when no aps env present) so callers
// can stash it on a CachedConfig once config loading is wired in.
//
// Logging path goes through w (defaults to os.Stderr) so the
// daemon-init test can capture it without a stderr stub.
//
// Fail-open: a missing profile is NOT an error — the daemon must
// still come up. Built-ins that need an agent_id token degrade
// gracefully (see config.ResolveTokens).
func initActiveProfile(w io.Writer) string {
	profile := config.LookupActiveProfile()
	if profile == "" {
		_, _ = fmt.Fprintln(w, "nervd: warning: no active aps profile (APS_PROFILE_ID unset); built-ins requiring <agent_id> will fall back to defaults")
	}
	return profile
}

const busShutdownTimeout = 5 * time.Second

func main() {
	os.Exit(run())
}

// run holds main's body so deferred cleanup executes before the
// process exits with runStateless's code.
func run() int {
	flag.Parse()
	startPprof()

	// Resolve aps active profile up-front so later config compilation
	// (builtins.Inject) sees a populated ActiveProfile field.
	profile := initActiveProfile(os.Stderr)

	// Load and compile config once. The bus is built from its bus
	// section, so config load precedes bus construction.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "nervd: failed to load config: %v\n", err)
		return 1
	}
	cc := config.Compile(cfg)
	builtins.Inject(cc.Config, cc.ByEvent)

	b := initBus(cfg.Bus)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), busShutdownTimeout)
		defer cancel()
		_ = b.Close(ctx)
	}()

	// crtx session recorder: folds published crtx events into envelope
	// JSONL that downstream indexers scan. Subscribed before any
	// dispatch so no emission is missed.
	stopRecorder := attachSessionRecorder(b)
	defer stopRecorder()

	matcher := dispatch.NewMatcher()
	runner := dispatch.NewRunnerWithBus(b)
	logger := &eventlog.JSONLLogger{}
	dispatcher := dispatch.New(cc, logger, matcher, runner)

	sockPath := os.Getenv("NERV_SOCKET")
	if sockPath != "" {
		// crtx consume side: foreign turns from the bus and from
		// foreign session scans route into the inbox. Daemon mode
		// only — stateless invocations never ingest.
		stopIngest := startIngest(cfg.Ingest, b, profile)
		defer stopIngest()

		runDaemon(sockPath, dispatcher)
		return 0
	}

	return runStateless(dispatcher)
}

// runStateless is the stdin→stdout single-shot mode. Input is either a
// Claude Code native hook event (settings.json wires nervd directly, so
// stdin carries hook_event_name JSON) or a nerv envelope (adapters).
// The response dialect follows the input dialect. Returns the process
// exit code.
func runStateless(d *dispatch.Dispatcher) int {
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, envelope.HardMaxEnvelopeSize+1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "nervd: failed to read stdin: %v\n", err)
		return 1
	}
	if int64(len(raw)) > envelope.HardMaxEnvelopeSize {
		fmt.Fprintf(os.Stderr, "nervd: input exceeds %d bytes\n", envelope.HardMaxEnvelopeSize)
		return 1
	}

	env, claudeEvent, err := decodeStateless(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nervd: failed to read event: %v\n", err)
		return 1
	}

	res, err := d.Dispatch(env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nervd: dispatch error: %v\n", err)
		if claudeEvent != "" {
			return 0 // fail-open: never wedge the calling CLI
		}
		return 1
	}

	if claudeEvent != "" {
		return respondClaude(os.Stdout, os.Stderr, claudeEvent, res.Decision)
	}

	if err := json.NewEncoder(os.Stdout).Encode(res.Decision); err != nil {
		fmt.Fprintf(os.Stderr, "nervd: failed to write decision: %v\n", err)
		return 1
	}
	return 0
}

// decodeStateless sniffs raw input: Claude-native hook JSON (marked by
// hook_event_name) translates through the claude adapter; anything else
// decodes as a nerv envelope. Returns the envelope plus the Claude
// event name ("" for envelope input).
func decodeStateless(raw []byte) (*envelope.Envelope, string, error) {
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, "", fmt.Errorf("decode: %w", err)
	}
	if name, ok := probe["hook_event_name"].(string); ok && name != "" {
		tr := &claude.Translator{}
		env, err := tr.ToEnvelope(probe)
		if err != nil {
			return nil, "", err
		}
		return env, name, nil
	}
	env, err := envelope.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, "", err
	}
	return env, "", nil
}

// respondClaude writes the merged decision in Claude's hook stdout
// contract and returns the exit code. Warn degrades to allow with the
// message on stderr. Translation failures fail open (exit 0) so a nerv
// bug never blocks the calling CLI.
func respondClaude(out, errw io.Writer, event string, d merger.Decision) int {
	cd := claude.Decision{Action: string(d.Action), Reason: d.Message}
	if d.Action == merger.Warn && d.Message != "" {
		_, _ = fmt.Fprintf(errw, "nervd: warn: %s\n", d.Message)
	}
	if d.Rewrite != nil {
		payload, err := json.Marshal(d.Rewrite)
		if err != nil {
			_, _ = fmt.Fprintf(errw, "nervd: marshal rewrite: %v\n", err)
			return 0
		}
		cd.Payload = payload
	}

	tr := &claude.Translator{}
	b, code, err := tr.FromDecision(cd, event)
	if err != nil {
		_, _ = fmt.Fprintf(errw, "nervd: respond: %v\n", err)
		return 0
	}
	if len(b) > 0 {
		_, _ = out.Write(b)
	}
	return code
}

// runDaemon starts the long-running ConnectRPC EventService server on
// the Unix domain socket at sockPath.
//
// Replaces the legacy NDJSON sockd loop: clients now speak
// nerv.v1.EventService over h2c instead of one-envelope-per-line JSON.
// Pipeline parity is preserved — the same dispatch.Dispatcher feeds
// both transports.
//
// Signal handling:
//   - SIGTERM / SIGINT → graceful shutdown.
//   - SIGHUP           → re-read hooks.yaml + .nerv/disabled.yaml and
//     swap the cached config atomically. In-flight
//     dispatches keep their pre-reload snapshot.
func runDaemon(sockPath string, d *dispatch.Dispatcher) {
	h, err := daemon.NewHandler(d)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nervd: handler: %v\n", err)
		os.Exit(1)
	}

	srv, err := daemon.New(daemon.Config{
		SocketPath: sockPath,
		Service:    h,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "nervd: server: %v\n", err)
		os.Exit(1)
	}

	// Split signal handling so SIGHUP triggers a config reload while
	// SIGTERM/SIGINT terminate the process.
	ctx, stop := signal.NotifyContext(context.Background(),
		syscall.SIGTERM, syscall.SIGINT,
	)
	defer stop()

	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	go reloadOnSIGHUP(ctx, hup, d)

	fmt.Fprintf(os.Stderr, "nervd: daemon mode (connectrpc), socket=%s\n", sockPath)

	if err := srv.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "nervd: %v\n", err)
		os.Exit(1)
	}
}

// reloadOnSIGHUP listens for SIGHUP and swaps the dispatcher's cached
// config in place. Errors are logged and the previous config keeps
// serving so a malformed reload doesn't take the daemon down.
func reloadOnSIGHUP(ctx context.Context, hup <-chan os.Signal, d *dispatch.Dispatcher) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-hup:
			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "nervd: reload: config load: %v\n", err)
				continue
			}
			cc := config.Compile(cfg)
			builtins.Inject(cc.Config, cc.ByEvent)
			d.Reload(cc)
			fmt.Fprintln(os.Stderr, "nervd: reload: config swapped")
		}
	}
}
