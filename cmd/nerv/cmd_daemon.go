//go:build !windows

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/core/xdg"
)

// daemonStatusRow is the structured `daemon status` report: the
// daemon's lifecycle state plus pid/socket detail when running.
type daemonStatusRow struct {
	State  string `table:"State"  json:"state"            yaml:"state"`
	PID    int    `table:"PID"    json:"pid,omitempty"    yaml:"pid,omitempty"`
	Socket string `table:"Socket" json:"socket,omitempty" yaml:"socket,omitempty"`
}

// renderDaemonStopPlan emits the structured dry-run preview for
// `nerv daemon stop`: which process would be signalled and which
// files cleaned up, without sending any signal.
func renderDaemonStopPlan(cmd *cobra.Command, pidfile, sock string) error {
	var effects []cli.Effect
	var warnings []string
	if pid, err := readPID(pidfile); err == nil && processAlive(pid) {
		effects = []cli.Effect{
			{
				Kind:       "signal",
				Target:     fmt.Sprintf("pid:%d", pid),
				Reversible: false,
				Detail:     "SIGTERM, then SIGKILL after 5s grace",
			},
			{Kind: "delete", Target: pidfile, Reversible: false, Detail: "clean up pidfile"},
			{Kind: "delete", Target: sock, Reversible: false, Detail: "clean up socket"},
		}
	} else {
		warnings = append(warnings,
			"daemon is not running; stop would fail with 'daemon not running (no pidfile)'")
	}
	return renderPlan(cmd, nil, effects, warnings...)
}

// daemonPaths resolves pidfile and socket paths.
//
// Pidfile lives under kit/xdg.StateDir("nerv"). Socket sits at
// $XDG_RUNTIME_DIR/nerv.sock (not in a per-tool subdir, since the
// socket is a single file resource and existing clients look here);
// on macOS / when unset, we fall back to os.TempDir().
func daemonPaths() (pidfile, sock string) {
	stateDir, err := xdg.StateDir("nerv")
	if err != nil {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".local", "state", "nerv")
	}
	pidfile = filepath.Join(stateDir, "daemon.pid")

	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	sock = filepath.Join(runtimeDir, "nerv.sock")
	return
}

// readPID reads the pidfile and returns the PID.
func readPID(pidfile string) (int, error) {
	data, err := os.ReadFile(pidfile)
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("invalid pid in %s: %w", pidfile, err)
	}
	return pid, nil
}

// processAlive checks whether the given PID is a running process.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks existence without sending a real signal.
	return proc.Signal(syscall.Signal(0)) == nil
}

func daemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the nerv daemon (nervd)",
	}

	cmd.AddCommand(
		daemonStartCmd(),
		daemonStopCmd(),
		daemonStatusCmd(),
		daemonReloadCmd(),
	)
	return cmd
}

func daemonStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start nervd in the background",
		Long: `Launch the nervd hot-path dispatcher as a detached background
process. Writes its PID to $XDG_STATE_HOME/nerv/daemon.pid and serves
hook events on $XDG_RUNTIME_DIR/nerv.sock. Idempotent: if the daemon
is already running, returns an error rather than spawning a duplicate.`,
		Annotations: map[string]string{
			"kit/side-effect": "interactive",
			"kit/idempotent":  "no",
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			pidfile, sock := daemonPaths()

			// Check if already running.
			if pid, err := readPID(pidfile); err == nil && processAlive(pid) {
				return fmt.Errorf("daemon already running (pid: %d)", pid)
			}

			// Locate nervd binary — same directory as nerv, or PATH.
			nervdBin, err := exec.LookPath("nervd")
			if err != nil {
				self, _ := os.Executable()
				candidate := filepath.Join(filepath.Dir(self), "nervd")
				if _, statErr := os.Stat(candidate); statErr == nil {
					nervdBin = candidate
				} else {
					return fmt.Errorf("nervd binary not found in PATH or next to nerv")
				}
			}

			// Ensure state dir exists for pidfile.
			if err := os.MkdirAll(filepath.Dir(pidfile), 0o700); err != nil {
				return fmt.Errorf("creating state dir: %w", err)
			}

			// Prepare log file.
			logDir := filepath.Dir(pidfile)
			logFile, err := os.OpenFile(
				filepath.Join(logDir, "daemon.log"),
				os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600,
			)
			if err != nil {
				return fmt.Errorf("opening log file: %w", err)
			}

			// Start nervd with socket path as env.
			child := exec.Command(nervdBin)
			child.Env = append(os.Environ(),
				"NERV_SOCKET="+sock,
			)
			child.Stdout = logFile
			child.Stderr = logFile
			// Detach from parent process group.
			child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

			if err := child.Start(); err != nil {
				_ = logFile.Close() // the start error is the one to report
				return fmt.Errorf("starting nervd: %w", err)
			}
			// The child holds its own descriptor; closing ours cannot lose its writes.
			_ = logFile.Close()

			// Write pidfile.
			pid := child.Process.Pid
			if err := os.WriteFile(pidfile, []byte(strconv.Itoa(pid)), 0o600); err != nil {
				return fmt.Errorf("writing pidfile: %w", err)
			}

			// Brief wait so nervd can bind before we report. The socket may
			// not exist yet; `nerv daemon status` reports its state.
			time.Sleep(200 * time.Millisecond)

			fmt.Printf("daemon started (pid: %d)\n", pid)
			return nil
		},
	}
}

func daemonStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running nervd daemon",
		Long: `Send SIGTERM to the running nervd, wait up to 5 seconds for a
graceful shutdown, then SIGKILL if still alive. Cleans up the pidfile
and socket on success. Use after config or binary upgrades that
SIGHUP reload cannot pick up.`,
		Annotations: map[string]string{
			"kit/side-effect": "destructive",
			"kit/idempotent":  "yes",
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			pidfile, sock := daemonPaths()

			if cli.IsDryRun(cmd) {
				return renderDaemonStopPlan(cmd, pidfile, sock)
			}

			pid, err := readPID(pidfile)
			if err != nil {
				return fmt.Errorf("daemon not running (no pidfile)")
			}

			proc, err := os.FindProcess(pid)
			if err != nil {
				return fmt.Errorf("finding process %d: %w", pid, err)
			}

			// Send SIGTERM.
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				// Process may already be dead.
				cleanup(pidfile, sock)
				fmt.Println("daemon stopped")
				return nil
			}

			// Wait up to 5s for graceful exit.
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if !processAlive(pid) {
					cleanup(pidfile, sock)
					fmt.Println("daemon stopped")
					return nil
				}
				time.Sleep(100 * time.Millisecond)
			}

			// Force kill.
			_ = proc.Signal(syscall.SIGKILL)
			time.Sleep(100 * time.Millisecond)
			cleanup(pidfile, sock)
			fmt.Println("daemon stopped (forced)")
			return nil
		},
	}
}

func daemonStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check daemon status",
		Long: `Report whether the nervd daemon is running by reading the pidfile,
checking the process is alive, and dialing the unix socket. Reports a
state of "stopped", "running", or "stopped (stale pidfile)" without
mutating any state. Safe to poll from monitoring scripts; honors
--format (table, json, yaml).`,
		Annotations: map[string]string{
			"kit/side-effect": "read",
			"kit/idempotent":  "yes",
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			pidfile, sock := daemonPaths()

			row := daemonStatusRow{State: "stopped"}
			if pid, err := readPID(pidfile); err == nil {
				switch {
				case !processAlive(pid):
					row = daemonStatusRow{
						State:  "stopped (stale pidfile)",
						PID:    pid,
						Socket: "",
					}
				default:
					row = daemonStatusRow{State: "running", PID: pid, Socket: "unavailable"}
					if conn, dErr := net.DialTimeout(
						"unix", sock, 500*time.Millisecond,
					); dErr == nil {
						_ = conn.Close() // probe connection; only the dial result matters
						row.Socket = "ok"
					}
				}
			}
			return renderResult(row)
		},
	}
}

func daemonReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Hot-reload hooks.yaml (sends SIGHUP to daemon)",
		Long: `Signal the running nervd to re-read hooks.yaml without restarting.
Use after editing hooks.yaml or the hooks.lock to apply changes
without dropping in-flight envelopes. Fails fast if no daemon is
running; pair with daemon status to check before issuing reload.`,
		Annotations: map[string]string{
			"kit/side-effect": "write",
			"kit/idempotent":  "yes",
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			pidfile, _ := daemonPaths()

			if cli.IsDryRun(cmd) {
				return renderDaemonReloadPlan(cmd, pidfile)
			}

			pid, err := readPID(pidfile)
			if err != nil {
				return fmt.Errorf("daemon not running (no pidfile)")
			}

			if !processAlive(pid) {
				return fmt.Errorf("daemon not running (stale pidfile, pid: %d)", pid)
			}

			proc, err := os.FindProcess(pid)
			if err != nil {
				return fmt.Errorf("finding process %d: %w", pid, err)
			}

			if err := proc.Signal(syscall.SIGHUP); err != nil {
				return fmt.Errorf("sending SIGHUP: %w", err)
			}

			fmt.Printf("reload signal sent (pid: %d)\n", pid)
			return nil
		},
	}
}

// renderDaemonReloadPlan emits the structured dry-run preview for
// `nerv daemon reload`: which process would receive SIGHUP, without
// delivering any signal. Mirrors renderDaemonStopPlan's warning
// behavior when no daemon is running.
func renderDaemonReloadPlan(cmd *cobra.Command, pidfile string) error {
	var effects []cli.Effect
	var warnings []string
	if pid, err := readPID(pidfile); err == nil && processAlive(pid) {
		effects = []cli.Effect{
			{
				Kind:       "signal",
				Target:     fmt.Sprintf("pid:%d", pid),
				Reversible: true,
				Detail:     "SIGHUP → re-read hooks.yaml (reload again to apply a reverted config)",
			},
		}
	} else {
		warnings = append(warnings,
			"daemon is not running; reload would fail with 'daemon not running (no pidfile)'")
	}
	return renderPlan(cmd, nil, effects, warnings...)
}

// cleanup removes pidfile and socket.
func cleanup(pidfile, sock string) {
	// Best-effort: either file may already be gone.
	_ = os.Remove(pidfile)
	_ = os.Remove(sock)
}
