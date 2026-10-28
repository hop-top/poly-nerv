package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/crtx"
)

func roundTrip(t *testing.T, b bus.Bus, topic string) {
	t.Helper()

	done := make(chan struct{})
	unsub := b.Subscribe(topic, func(_ context.Context, _ bus.Event) error {
		close(done)
		return nil
	})
	defer unsub()

	if err := b.Publish(context.Background(), bus.NewEvent(
		bus.Topic(topic), "nervd.test", map[string]string{"k": "v"},
	)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not receive event")
	}
}

func readJSONLTopics(t *testing.T, path string) []string {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open sink file: %v", err)
	}
	defer func() { _ = f.Close() }() // read-only

	var topics []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec struct {
			Topic string `json:"topic"`
		}
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("bad jsonl line %q: %v", sc.Text(), err)
		}
		topics = append(topics, rec.Topic)
	}
	return topics
}

// TestBuildBus_DefaultMemory verifies the zero-value config produces a
// working in-memory bus.
func TestBuildBus_DefaultMemory(t *testing.T) {
	b := buildBus(config.BusConfig{})
	defer func() { _ = b.Close(context.Background()) }()
	roundTrip(t, b, "nervd.test.event.published")
}

// TestBuildBus_ExplicitJSONLSink verifies a configured jsonl sink
// receives published events as JSONL lines.
func TestBuildBus_ExplicitJSONLSink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	b := buildBus(config.BusConfig{
		Adapter: "memory",
		Sinks:   []config.BusSink{{Type: "jsonl", Path: path}},
	})

	roundTrip(t, b, "nervd.test.event.published")
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	topics := readJSONLTopics(t, path)
	if len(topics) != 1 || topics[0] != "nervd.test.event.published" {
		t.Fatalf("sink file topics = %v, want single nervd.test.event.published", topics)
	}
}

// TestBuildBus_JSONLAdapterShorthand verifies `adapter: jsonl` means
// memory transport + an implicit jsonl sink at the default state path.
func TestBuildBus_JSONLAdapterShorthand(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	b := buildBus(config.BusConfig{Adapter: "jsonl"})
	roundTrip(t, b, "nervd.test.event.published")
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	path := filepath.Join(stateHome, "nerv", "events", "bus.jsonl")
	topics := readJSONLTopics(t, path)
	if len(topics) != 1 {
		t.Fatalf("default sink file %s topics = %v, want 1 entry", path, topics)
	}
}

// TestBuildBus_SQLiteAdapter verifies the sqlite adapter materializes
// its database under the default state path and still delivers.
func TestBuildBus_SQLiteAdapter(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	b := buildBus(config.BusConfig{Adapter: "sqlite"})
	defer func() { _ = b.Close(context.Background()) }()

	roundTrip(t, b, "nervd.test.event.published")

	path := filepath.Join(stateHome, "nerv", "events", "bus.sqlite")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("sqlite adapter db missing at %s: %v", path, err)
	}
}

// TestBuildBus_NetworkWithoutEndpointFallsBack verifies fail-open:
// a network adapter without an endpoint degrades to memory instead of
// taking the daemon down.
func TestBuildBus_NetworkWithoutEndpointFallsBack(t *testing.T) {
	b := buildBus(config.BusConfig{Adapter: "network"})
	defer func() { _ = b.Close(context.Background()) }()
	roundTrip(t, b, "nervd.test.event.published")
}

// TestBuildBus_UnknownAdapterFallsBack verifies defense-in-depth for
// values the config validator should have rejected.
func TestBuildBus_UnknownAdapterFallsBack(t *testing.T) {
	b := buildBus(config.BusConfig{Adapter: "bogus"})
	defer func() { _ = b.Close(context.Background()) }()
	roundTrip(t, b, "nervd.test.event.published")
}

// TestBuildBus_UnsupportedSinkTypesSkipped verifies sqlite/network
// sink entries (validated shapes without kit sink implementations yet)
// are skipped without breaking delivery or supported sinks.
func TestBuildBus_UnsupportedSinkTypesSkipped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	b := buildBus(config.BusConfig{
		Adapter: "memory",
		Sinks: []config.BusSink{
			{Type: "sqlite", Path: filepath.Join(t.TempDir(), "x.db")},
			{Type: "network"},
			{Type: "jsonl", Path: path},
		},
	})

	roundTrip(t, b, "nervd.test.event.published")
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if topics := readJSONLTopics(t, path); len(topics) != 1 {
		t.Fatalf("jsonl sink topics = %v, want 1 entry", topics)
	}
}

// TestInitBusFromConfig verifies main's init path honors the loaded
// bus config and registers the package-level daemon bus.
func TestInitBusFromConfig(t *testing.T) {
	prev := daemonBus
	defer func() { daemonBus = prev }()
	daemonBus = nil

	path := filepath.Join(t.TempDir(), "events.jsonl")
	b := initBus(config.BusConfig{
		Sinks: []config.BusSink{{Type: "jsonl", Path: path}},
	})

	if Bus() != b {
		t.Fatal("Bus() != initBus return value")
	}

	roundTrip(t, b, "nervd.test.event.published")
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if topics := readJSONLTopics(t, path); len(topics) != 1 {
		t.Fatalf("sink topics = %v, want 1 entry", topics)
	}
}

// TestNervStateDir_XDGFallback pins the default state root used for
// bus artifacts.
func TestNervStateDir_XDGFallback(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")
	if got := nervStateDir(); got != filepath.Join("/custom/state", "nerv") {
		t.Errorf("nervStateDir with XDG_STATE_HOME = %q", got)
	}

	t.Setenv("XDG_STATE_HOME", "")
	got := nervStateDir()
	if !strings.HasSuffix(got, filepath.Join(".local", "state", "nerv")) {
		t.Errorf("nervStateDir fallback = %q, want ~/.local/state/nerv", got)
	}
}

// TestAttachSessionRecorder verifies the daemon folds published crtx
// events into envelope JSONL under <state>/nerv/sessions/.
func TestAttachSessionRecorder(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	b := buildBus(config.BusConfig{})
	defer func() { _ = b.Close(context.Background()) }()

	unsub := attachSessionRecorder(b)
	defer unsub()

	if err := b.Publish(context.Background(), bus.NewEvent(
		bus.Topic(crtx.TopicUserReceived), crtx.BusSource,
		crtx.UserReceivedPayload{
			EnvelopeID: "sess-daemon",
			TurnID:     "t-1",
			Content:    []any{crtx.NewTextPart("hello")},
		},
	)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	path := filepath.Join(stateHome, "nerv", "sessions", "sess-daemon.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("session envelope not written to %s: %v", path, err)
	}
	var env struct {
		CrtxVersion string `json:"crtx_version"`
		ID          string `json:"id"`
	}
	if err := json.Unmarshal(data[:len(data)-1], &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.CrtxVersion != crtx.Version || env.ID != "sess-daemon" {
		t.Errorf("envelope = %+v", env)
	}
}
