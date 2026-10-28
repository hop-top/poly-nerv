//go:build !short && !windows

package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"hop.top/nerv/internal/events"
)

func TestFSWatcher_Start_Stop(t *testing.T) {
	dir := t.TempDir()
	w := NewFSWatcher(dir)

	err := w.Start()
	if err != nil {
		t.Skipf("skipping: watcher binary not available: %v", err)
	}

	// Double start should fail.
	if err := w.Start(); err == nil {
		t.Error("expected error on double start")
	}

	if err := w.Stop(); err != nil {
		t.Errorf("Stop() error: %v", err)
	}

	// Double stop should be safe.
	if err := w.Stop(); err != nil {
		t.Errorf("second Stop() error: %v", err)
	}
}

func TestFSWatcher_FileChange(t *testing.T) {
	dir := t.TempDir()
	w := NewFSWatcher(dir)

	err := w.Start()
	if err != nil {
		t.Skipf("skipping: watcher binary not available: %v", err)
	}
	defer func() { _ = w.Stop() }() // teardown; Stop errors don't affect the assertions

	// Give the watcher time to initialize.
	time.Sleep(200 * time.Millisecond)

	// Create a file to trigger an event.
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Wait for the event with a timeout.
	select {
	case ev := <-w.Events():
		if ev.Event != events.EventFileChanged {
			t.Errorf("event = %q, want FileChanged", ev.Event)
		}
	case <-time.After(5 * time.Second):
		t.Error("timed out waiting for FileChanged event")
	}
}

func TestFSWatcher_Classify(t *testing.T) {
	w := NewFSWatcher("/tmp")

	tests := []struct {
		path string
		want events.Event
	}{
		{"/home/user/.codex/config.toml", events.EventConfigChange},
		{"/project/.codex/hooks.json", events.EventConfigChange},
		{"/project/src/main.go", events.EventFileChanged},
		{"/project/README.md", events.EventFileChanged},
	}

	for _, tt := range tests {
		got := w.classify(tt.path)
		if got.Event != tt.want {
			t.Errorf("classify(%q) = %q, want %q", tt.path, got.Event, tt.want)
		}
	}
}
