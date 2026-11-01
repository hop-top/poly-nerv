package main

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/viper"
)

// captureStdout runs fn with os.Stdout redirected into a pipe and
// returns what fn wrote. Commands here render straight to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	func() {
		defer func() {
			os.Stdout = orig
			_ = w.Close()
		}()
		fn()
	}()
	return <-done
}

// setFormat sets the process-global viper --format for one test.
func setFormat(t *testing.T, format string) {
	t.Helper()
	prev := viper.GetString("format")
	viper.Set("format", format)
	t.Cleanup(func() { viper.Set("format", prev) })
}

// isolateEnv points HOME and every XDG dir at fresh temp dirs and
// chdirs into an empty project dir, so no real user state is read.
func isolateEnv(t *testing.T) (project string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root+"/xdg-config")
	t.Setenv("XDG_DATA_HOME", root+"/xdg-data")
	t.Setenv("XDG_STATE_HOME", root+"/xdg-state")
	t.Setenv("XDG_RUNTIME_DIR", root+"/xdg-run")
	project = t.TempDir()
	t.Chdir(project)
	return project
}
