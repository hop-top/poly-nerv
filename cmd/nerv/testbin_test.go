package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	nervBinOnce sync.Once
	nervBinPath string
	nervBinErr  error
	nervBinDir  string
)

// TestMain isolates the process from the invoking user's coverage
// env and tears down the shared nerv binary. When NERV_COVERDIR is
// set, the subprocess binary is built with -cover and its profiles
// land in that dir (via GOCOVERDIR), so the subprocess tests count
// toward coverage once merged with `go tool covdata`.
func TestMain(m *testing.M) {
	if dir := os.Getenv("NERV_COVERDIR"); dir != "" {
		abs, err := filepath.Abs(dir)
		if err == nil {
			_ = os.MkdirAll(abs, 0o755)
			os.Setenv("GOCOVERDIR", abs)
		}
	} else {
		// An inherited GOCOVERDIR would make a plain binary noisy
		// nowhere, but keep the child env deterministic.
		os.Unsetenv("GOCOVERDIR")
	}
	code := m.Run()
	if nervBinDir != "" {
		_ = os.RemoveAll(nervBinDir)
	}
	os.Exit(code)
}

// buildNerv returns the path of a nerv binary built once per test
// process and shared by every subprocess test.
func buildNerv(t *testing.T) string {
	t.Helper()
	nervBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "nerv-bin-")
		if err != nil {
			nervBinErr = err
			return
		}
		nervBinDir = dir
		nervBinPath = filepath.Join(dir, "nerv-test")
		args := []string{"build", "-buildvcs=false"}
		if os.Getenv("NERV_COVERDIR") != "" {
			args = append(args, "-cover", "-coverpkg=hop.top/nerv/...")
		}
		args = append(args, "-o", nervBinPath, ".")
		if out, err := exec.Command("go", args...).CombinedOutput(); err != nil {
			nervBinErr = &buildError{err: err, out: string(out)}
		}
	})
	if nervBinErr != nil {
		t.Fatalf("build failed: %v", nervBinErr)
	}
	return nervBinPath
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }
