package codex

import (
	"bufio"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"hop.top/nerv/internal/events"
)

// FSWatcher spawns fswatch or inotifywait to detect file changes
// and emits FileChanged / ConfigChange synthetic events.
//
// Only active in daemon mode (not stateless hook execution).
type FSWatcher struct {
	dir    string
	events chan SyntheticEvent
	cmd    *exec.Cmd
	done   chan struct{}
	mu     sync.Mutex
}

// NewFSWatcher creates a watcher for the given project directory.
func NewFSWatcher(dir string) *FSWatcher {
	return &FSWatcher{
		dir:    dir,
		events: make(chan SyntheticEvent, 64),
	}
}

// Start launches the background file watcher process.
// Returns an error if no suitable watcher binary is found.
func (w *FSWatcher) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cmd != nil {
		return fmt.Errorf("fswatch: already running")
	}

	bin, args, err := w.buildCommand()
	if err != nil {
		return err
	}

	// Fresh done channel per Start to avoid panic on restart.
	done := make(chan struct{})
	w.done = done

	w.cmd = exec.Command(bin, args...)
	stdout, err := w.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("fswatch: stdout pipe: %w", err)
	}

	if err := w.cmd.Start(); err != nil {
		return fmt.Errorf("fswatch: start: %w", err)
	}

	go func() {
		defer close(done)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			path := strings.TrimSpace(scanner.Text())
			if path == "" {
				continue
			}
			// Non-blocking send to avoid deadlocking Stop().
			select {
			case w.events <- w.classify(path):
			default:
				// Buffer full — drop event rather than block.
			}
		}
	}()

	return nil
}

// Stop terminates the background watcher process.
func (w *FSWatcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.cmd == nil || w.cmd.Process == nil {
		return nil
	}

	if err := w.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("fswatch: kill: %w", err)
	}
	// Reap the process to avoid zombies.
	_ = w.cmd.Wait()
	<-w.done
	w.cmd = nil
	return nil
}

// Events returns the channel that receives synthetic events.
func (w *FSWatcher) Events() <-chan SyntheticEvent {
	return w.events
}

// buildCommand selects the right watcher binary and args for the platform.
func (w *FSWatcher) buildCommand() (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		bin, err := exec.LookPath("fswatch")
		if err != nil {
			return "", nil, fmt.Errorf(
				"fswatch: not found; install via: brew install fswatch",
			)
		}
		return bin, []string{
			"--recursive",
			"--exclude", `.git`,
			w.dir,
		}, nil

	case "linux":
		bin, err := exec.LookPath("inotifywait")
		if err != nil {
			return "", nil, fmt.Errorf(
				"inotifywait: not found; install via: apt install inotify-tools",
			)
		}
		return bin, []string{
			"-m", "-r",
			"--exclude", `.git`,
			"-e", "modify,create,delete",
			"--format", "%w%f",
			w.dir,
		}, nil

	default:
		return "", nil, fmt.Errorf("fswatch: unsupported platform %q", runtime.GOOS)
	}
}

// classify determines whether a file path change is a ConfigChange
// or a regular FileChanged event.
func (w *FSWatcher) classify(path string) SyntheticEvent {
	base := filepath.Base(path)

	configFiles := map[string]bool{
		"config.toml": true,
		"hooks.json":  true,
	}

	if configFiles[base] {
		return SyntheticEvent{
			Event:  events.EventConfigChange,
			Source: "fswatch",
		}
	}

	return SyntheticEvent{
		Event:  events.EventFileChanged,
		Source: "fswatch",
	}
}
