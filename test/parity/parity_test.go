//go:build pending_first_tag

// Excluded from the default build until hop.top/nerv/sdk/go ships its
// first tag via release-please. Imports the SDK module across the
// root↔sdk/go boundary, which only resolves through the module proxy
// once tagged. Run locally with: go test -tags pending_first_tag ./...

// Package parity is the cross-SDK wire-compatibility smoke harness for
// nerv.
//
// It spawns an in-process daemon over a Unix socket with a fake
// EventService that returns deterministic markers for both the List
// catalog and the Dispatch decision. Each TestParity_<lang> drives one
// SDK at that socket and asserts both markers round-trip.
//
// Full surface = List + Dispatch. Close is a client-side cleanup
// concern verified by per-SDK unit tests, not here.
//
// SDK coverage:
//   - Go        — in-process via sdk/go.
//   - TypeScript — shells out to node parity_client.mjs.
//   - Python    — shells out to sdk/py/.venv/bin/python3 parity_client.py.
//   - Rust      — pre-built cargo example sdk/rs/examples/parity_client.rs.
//   - PHP       — shells out to php parity_client.php; dials the
//     daemon via HopTop\Nerv\Client::fromSocket (real gRPC over PECL
//     grpc). Skips at runtime ONLY when the PECL grpc extension is
//     not loaded on the runner (exit code 3). CI installs ext-grpc
//     via shivammathur/setup-php so the test runs unskipped.
//
// macOS sun_path is 104 bytes. We use os.MkdirTemp (lands under
// /var/folders or /tmp with short prefixes) instead of t.TempDir
// (which embeds the test name and can overflow).
package parity

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	nervv1 "hop.top/nerv/gen/go/nerv/v1"
	"hop.top/nerv/internal/daemon"
	nervsdk "hop.top/nerv/sdk/go"
)

// parityMarker is the deterministic Decision.Message the fake
// EventService returns for the known envelope. Every SDK test asserts
// stdout/decision contains this string.
const parityMarker = "parity-marker"

// parityListMarker is the deterministic EventDescriptor.Name the fake
// EventService returns from List. The full-surface parity contract is:
// every SDK client calls List then Dispatch and surfaces both markers
// so the harness asserts both round-trip cleanly.
const parityListMarker = "PreToolUse"

// fakeEventService returns a fixed Decision for an envelope whose
// event=="PreToolUse" and cli.name=="claude" — the parity contract.
// Any other envelope returns ACTION_BLOCK with a "no-match" message so
// a misconfigured client trips the assertion loudly.
type fakeEventService struct{}

func (f *fakeEventService) Dispatch(
	_ context.Context, req *nervv1.DispatchRequest,
) (*nervv1.DispatchResponse, error) {
	env := req.GetEnvelope()
	if env == nil {
		return &nervv1.DispatchResponse{Decision: &nervv1.Decision{
			Action:  nervv1.Action_ACTION_BLOCK,
			Message: "no-envelope",
		}}, nil
	}
	if env.GetEvent() == "PreToolUse" && env.GetCli().GetName() == "claude" {
		return &nervv1.DispatchResponse{Decision: &nervv1.Decision{
			Action:  nervv1.Action_ACTION_ALLOW,
			Message: parityMarker,
		}}, nil
	}
	return &nervv1.DispatchResponse{Decision: &nervv1.Decision{
		Action:  nervv1.Action_ACTION_BLOCK,
		Message: "no-match",
	}}, nil
}

func (f *fakeEventService) List(
	_ context.Context, _ *nervv1.ListRequest,
) (*nervv1.ListResponse, error) {
	return &nervv1.ListResponse{
		Events: []*nervv1.EventDescriptor{
			{Name: parityListMarker, Category: "tool", Origin: "native", Blocking: true},
		},
	}, nil
}

// startDaemon spins up internal/daemon.Server on a short UDS path
// with the parity fake EventService and returns the socket path
// plus a cleanup func that stops the server and removes the dir.
func startDaemon(t *testing.T) (string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("", "nerv-parity-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	sock := filepath.Join(dir, "n.sock")

	srv, err := daemon.New(daemon.Config{
		SocketPath: sock,
		Service:    &fakeEventService{},
	})
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("daemon.New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()

	if err := waitForSocket(sock); err != nil {
		cancel()
		select {
		case serveErr := <-errCh:
			_ = os.RemoveAll(dir)
			t.Fatalf("waitForSocket: %v; Serve error: %v", err, serveErr)
		case <-time.After(time.Second):
		}
		_ = os.RemoveAll(dir)
		t.Fatalf("waitForSocket: %v", err)
	}

	cleanup := func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(2 * time.Second):
		}
		_ = os.RemoveAll(dir)
	}
	return sock, cleanup
}

func waitForSocket(path string) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return errors.New("socket did not appear")
}

// repoRoot walks up from this test file until it finds go.mod.
// Needed to locate sibling sdk/{py,ts}/ trees from the test binary.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found from " + dir)
		}
		dir = parent
	}
}

// makeEnvelope returns the canonical parity envelope. Kept in one
// place so Go/TS/Py tests stay aligned.
func makeEnvelope() *nervv1.Envelope {
	return &nervv1.Envelope{
		NervVersion: "1.0.0-draft",
		Event:       "PreToolUse",
		EventId:     "parity-event",
		Cli: &nervv1.CLIInfo{
			Name:    "claude",
			Version: "0.0.0",
		},
		Session: &nervv1.SessionInfo{
			Id:  "parity-session",
			Cwd: "/tmp",
		},
	}
}

// TestParity_Go drives the Go SDK against the in-process daemon.
// Full-surface: calls List then Dispatch and asserts both markers.
func TestParity_Go(t *testing.T) {
	sock, cleanup := startDaemon(t)
	defer cleanup()

	client := nervsdk.NewUDS(sock)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	events, err := client.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("List: empty event catalog")
	}
	if got, want := events[0].GetName(), parityListMarker; got != want {
		t.Errorf("events[0].Name = %q, want %q", got, want)
	}

	decision, err := client.Dispatch(ctx, makeEnvelope())
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if decision == nil {
		t.Fatal("Dispatch: nil decision")
	}
	if got, want := decision.GetMessage(), parityMarker; got != want {
		t.Errorf("decision.Message = %q, want %q", got, want)
	}
	if got, want := decision.GetAction(), nervv1.Action_ACTION_ALLOW; got != want {
		t.Errorf("decision.Action = %v, want %v", got, want)
	}
}

// TestParity_TypeScript shells out to node running a small built TS
// script that uses sdk/ts to dispatch the parity envelope and prints
// the decision message on stdout.
//
// Node ESM resolves bare specifiers ("@bufbuild/protobuf") by walking
// up from the script's directory looking for node_modules/. The script
// lives at test/parity/parity_client.mjs and needs sdk/ts's installed
// deps, so we create a symlink test/parity/node_modules pointing at
// sdk/ts/node_modules (idempotent; gitignored).
//
// Skips when:
//   - node isn't on PATH
//   - sdk/ts/dist hasn't been built (pnpm install + pnpm run build)
func TestParity_TypeScript(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not on PATH; skipping TS parity")
	}
	root := repoRoot(t)
	tsDist := filepath.Join(root, "sdk", "ts", "dist", "index.js")
	if _, err := os.Stat(tsDist); err != nil {
		t.Skipf("sdk/ts/dist not built (run pnpm install && pnpm run build): %v", err)
	}
	tsNodeModules := filepath.Join(root, "sdk", "ts", "node_modules")
	if _, err := os.Stat(tsNodeModules); err != nil {
		t.Skipf("sdk/ts/node_modules missing (run pnpm install): %v", err)
	}
	script := filepath.Join(root, "test", "parity", "parity_client.mjs")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("parity_client.mjs missing: %v", err)
	}

	// Make a symlink so the script's ESM bare imports resolve. If a
	// symlink already exists (prior run), leave it alone. If something
	// else is at that path, fail loudly rather than silently overwrite.
	parityNodeModules := filepath.Join(root, "test", "parity", "node_modules")
	if info, err := os.Lstat(parityNodeModules); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s exists and is not a symlink; refusing to clobber", parityNodeModules)
		}
	} else if os.IsNotExist(err) {
		if err := os.Symlink(tsNodeModules, parityNodeModules); err != nil {
			t.Fatalf("symlink %s -> %s: %v", parityNodeModules, tsNodeModules, err)
		}
	} else {
		t.Fatalf("stat %s: %v", parityNodeModules, err)
	}

	sock, cleanup := startDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", script)
	cmd.Env = append(os.Environ(), "NERV_SOCKET="+sock)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node %s: %v\noutput:\n%s", script, err, string(out))
	}
	if !strings.Contains(string(out), parityMarker) {
		t.Fatalf("TS output missing dispatch marker %q:\n%s", parityMarker, string(out))
	}
	if !strings.Contains(string(out), parityListMarker) {
		t.Fatalf("TS output missing list marker %q:\n%s", parityListMarker, string(out))
	}
}

// TestParity_Python shells out to sdk/py/.venv/bin/python3 running a
// small script that uses sdk/py to dispatch the parity envelope and
// prints the decision message on stdout.
//
// Skips when sdk/py/.venv/bin/python3 is missing (venv not provisioned).
func TestParity_Python(t *testing.T) {
	root := repoRoot(t)
	py := filepath.Join(root, "sdk", "py", ".venv", "bin", "python3")
	if _, err := os.Stat(py); err != nil {
		t.Skipf("sdk/py/.venv/bin/python3 missing (run: python3.12 -m venv sdk/py/.venv && sdk/py/.venv/bin/pip install -e sdk/py): %v", err)
	}
	script := filepath.Join(root, "test", "parity", "parity_client.py")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("parity_client.py missing: %v", err)
	}

	sock, cleanup := startDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, py, script)
	cmd.Env = append(os.Environ(), "NERV_SOCKET="+sock)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python %s: %v\noutput:\n%s", script, err, string(out))
	}
	if !strings.Contains(string(out), parityMarker) {
		t.Fatalf("Python output missing dispatch marker %q:\n%s", parityMarker, string(out))
	}
	if !strings.Contains(string(out), parityListMarker) {
		t.Fatalf("Python output missing list marker %q:\n%s", parityListMarker, string(out))
	}
}

// rustBuildOnce guards the cargo build of sdk/rs/examples/parity_client
// so the work happens at most once per test-package invocation. Each
// TestParity_Rust call after the first reuses the artifact at
// sdk/rs/target/debug/examples/parity_client.
var (
	rustBuildOnce sync.Once
	rustBuildErr  error
)

// buildRustParityClient runs `cargo build --example parity_client` once
// per test binary. Skips silently when cargo is absent so the test can
// degrade gracefully on runners without rust installed.
func buildRustParityClient(t *testing.T, manifest string) error {
	t.Helper()
	rustBuildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "cargo", "build",
			"--example", "parity_client",
			"--manifest-path", manifest,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			rustBuildErr = errors.New("cargo build --example parity_client: " +
				err.Error() + "\noutput:\n" + string(out))
		}
	})
	return rustBuildErr
}

// TestParity_Rust shells out to a pre-built cargo example that uses
// sdk/rs to call List + Dispatch against the parity envelope and prints
// both markers on stdout.
//
// Skips when:
//   - cargo isn't on PATH
//   - sdk/rs/Cargo.toml is missing (unlikely; sanity check)
func TestParity_Rust(t *testing.T) {
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo not on PATH; skipping Rust parity")
	}
	root := repoRoot(t)
	manifest := filepath.Join(root, "sdk", "rs", "Cargo.toml")
	if _, err := os.Stat(manifest); err != nil {
		t.Skipf("sdk/rs/Cargo.toml missing: %v", err)
	}

	if err := buildRustParityClient(t, manifest); err != nil {
		t.Fatalf("buildRustParityClient: %v", err)
	}

	bin := filepath.Join(root, "sdk", "rs", "target", "debug", "examples", "parity_client")
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("parity_client binary missing after build: %v", err)
	}

	sock, cleanup := startDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = append(os.Environ(), "NERV_SOCKET="+sock)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\noutput:\n%s", bin, err, string(out))
	}
	if !strings.Contains(string(out), parityMarker) {
		t.Fatalf("Rust output missing dispatch marker %q:\n%s", parityMarker, string(out))
	}
	if !strings.Contains(string(out), parityListMarker) {
		t.Fatalf("Rust output missing list marker %q:\n%s", parityListMarker, string(out))
	}
}

// phpInstallOnce guards `composer install --working-dir=sdk/php` so
// the work happens at most once per test-package invocation.
var (
	phpInstallOnce sync.Once
	phpInstallErr  error
)

// ensurePHPVendor runs composer install once if sdk/php/vendor/ is
// missing. Skips silently when composer is absent.
func ensurePHPVendor(t *testing.T, sdkDir string) error {
	t.Helper()
	vendor := filepath.Join(sdkDir, "vendor")
	if _, err := os.Stat(vendor); err == nil {
		return nil
	}
	if _, err := exec.LookPath("composer"); err != nil {
		return errors.New("composer not on PATH and sdk/php/vendor missing")
	}
	phpInstallOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "composer", "install",
			"--no-interaction", "--prefer-dist",
			"--working-dir="+sdkDir,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			phpInstallErr = errors.New("composer install: " + err.Error() +
				"\noutput:\n" + string(out))
		}
	})
	return phpInstallErr
}

// TestParity_PHP shells out to `php test/parity/parity_client.php`
// against the in-process daemon. Asserts stdout contains both markers.
//
// Skips when:
//   - php isn't on PATH
//   - sdk/php/vendor/ is missing AND composer isn't on PATH
//   - PECL grpc extension is not loaded on the runner — parity_client.php
//     exits 3 with a "ext-grpc not loaded" diagnostic. CI installs the
//     extension via shivammathur/setup-php so this skip only fires on
//     developer workstations without ext-grpc installed.
func TestParity_PHP(t *testing.T) {
	if _, err := exec.LookPath("php"); err != nil {
		t.Skip("php not on PATH; skipping PHP parity")
	}
	root := repoRoot(t)
	sdkDir := filepath.Join(root, "sdk", "php")
	if err := ensurePHPVendor(t, sdkDir); err != nil {
		t.Skipf("sdk/php/vendor unavailable: %v", err)
	}
	script := filepath.Join(root, "test", "parity", "parity_client.php")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("parity_client.php missing: %v", err)
	}

	sock, cleanup := startDaemon(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "php", script)
	cmd.Env = append(os.Environ(), "NERV_SOCKET="+sock)
	out, err := cmd.CombinedOutput()
	// Exit code 3 = PECL grpc extension not loaded on the runner.
	// Skip on dev workstations; CI installs the extension via
	// shivammathur/setup-php so this branch never fires there.
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 3 {
		t.Skipf("PECL grpc extension not loaded: %s", string(out))
	}
	if err != nil {
		t.Fatalf("php %s: %v\noutput:\n%s", script, err, string(out))
	}
	if !strings.Contains(string(out), parityMarker) {
		t.Fatalf("PHP output missing dispatch marker %q:\n%s", parityMarker, string(out))
	}
	if !strings.Contains(string(out), parityListMarker) {
		t.Fatalf("PHP output missing list marker %q:\n%s", parityListMarker, string(out))
	}
}
