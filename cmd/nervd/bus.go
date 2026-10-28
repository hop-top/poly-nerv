package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/crtx"
)

// nervStateDir returns the producer's state root:
// $XDG_STATE_HOME/nerv (fallback ~/.local/state/nerv). Bus artifacts
// live under its events/ subtree; crtx session envelopes under
// sessions/ (see docs/architecture/crtx-producer.md §state layout).
func nervStateDir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "nerv")
}

// busEventsDir is the default directory for adapter/sink artifacts.
func busEventsDir() string {
	return filepath.Join(nervStateDir(), "events")
}

// sessionsDir is the crtx envelope directory consumers scan.
func sessionsDir() string {
	return filepath.Join(nervStateDir(), "sessions")
}

// attachSessionRecorder subscribes the crtx session recorder on b,
// folding published crtx events into envelope JSONL under
// <state>/nerv/sessions/ (one file per session, stem JSONLStore
// convention). Returns the combined unsubscribe.
func attachSessionRecorder(b bus.Bus) func() {
	store := crtx.NewSessionStore(sessionsDir())
	return crtx.NewRecorder(store).Attach(b)
}

// buildBus constructs the daemon bus from the loaded hooks.yaml bus
// section. Every failure path degrades to the in-memory adapter with
// a logged warning: the bus is a side channel and nervd MUST keep
// dispatching (spec/failure-modes.md).
//
// Adapter semantics:
//   - "" / "memory": in-process pub/sub, no I/O on the hot path.
//   - "jsonl": shorthand for memory + one implicit jsonl sink at
//     <state>/events/bus.jsonl.
//   - "sqlite": kit SQLiteAdapter at <state>/events/bus.sqlite
//     (cross-process delivery via polling).
//   - "network": kit network transport dialing bus.network.endpoint.
//
// Configured sinks fan out from the chosen adapter via kit's TeeBus.
func buildBus(bc config.BusConfig) bus.Bus {
	base := coreBus(bc)

	sinks := buildSinks(bc)
	if len(sinks) == 0 {
		return base
	}
	return bus.NewTeeBus(base, sinks, func(err error) {
		log.Printf("nervd: bus sink error (fail-open): %v", err)
	})
}

// coreBus selects the transport adapter.
func coreBus(bc config.BusConfig) bus.Bus {
	switch bc.Adapter {
	case "", "memory", "jsonl":
		// jsonl is memory transport; its implicit sink is added by
		// buildSinks.
		return bus.New()

	case "sqlite":
		path := filepath.Join(busEventsDir(), "bus.sqlite")
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			log.Printf("nervd: bus sqlite dir %s: %v; falling back to memory", filepath.Dir(path), err)
			return bus.New()
		}
		a, err := bus.NewSQLiteAdapter(path)
		if err != nil {
			log.Printf("nervd: bus sqlite adapter %s: %v; falling back to memory", path, err)
			return bus.New()
		}
		return bus.New(bus.WithAdapter(a))

	case "network":
		if bc.Network.Endpoint == "" {
			log.Printf("nervd: bus.adapter=network without endpoint; falling back to memory")
			return bus.New()
		}
		return bus.New(bus.WithNetwork(bc.Network.Endpoint))

	default:
		log.Printf("nervd: unknown bus.adapter %q; falling back to memory", bc.Adapter)
		return bus.New()
	}
}

// buildSinks materializes the configured sink list, plus the implicit
// jsonl sink implied by `adapter: jsonl`. Unsupported entries are
// skipped with a logged reason — never fatal.
func buildSinks(bc config.BusConfig) []bus.Sink {
	var sinks []bus.Sink

	if bc.Adapter == "jsonl" {
		path := filepath.Join(busEventsDir(), "bus.jsonl")
		if s, err := jsonlSink(path); err != nil {
			log.Printf("nervd: default jsonl sink %s: %v; skipping", path, err)
		} else {
			sinks = append(sinks, s)
		}
	}

	for i, sc := range bc.Sinks {
		switch sc.Type {
		case "jsonl":
			s, err := jsonlSink(sc.Path)
			if err != nil {
				log.Printf("nervd: bus.sinks[%d] jsonl %s: %v; skipping", i, sc.Path, err)
				continue
			}
			if sc.Rotate.MaxBytes > 0 || sc.Rotate.MaxFiles > 0 {
				log.Printf("nervd: bus.sinks[%d] rotate: not yet supported; appending without rotation", i)
			}
			sinks = append(sinks, s)

		case "stdout":
			sinks = append(sinks, bus.NewStdoutSink())

		default:
			// sqlite / network sink shapes validate at config load but
			// have no kit sink implementation yet.
			log.Printf("nervd: bus.sinks[%d] type %q not yet supported; skipping", i, sc.Type)
		}
	}
	return sinks
}

// jsonlSink opens an append-mode jsonl sink, creating parent
// directories as needed.
func jsonlSink(path string) (bus.Sink, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	return bus.NewJSONLSinkFile(path)
}
