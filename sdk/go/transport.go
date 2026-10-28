package nervsdk

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

// EnvSocket is the environment variable consulted by DefaultSocketPath
// to override the XDG-derived default. Matches the nerv daemon
// convention used by cmd/nervd and cmd/nerv daemon.
const EnvSocket = "NERV_SOCKET"

// DefaultSocketPath returns the conventional UDS path for a local
// nerv daemon: $NERV_SOCKET if set, otherwise $XDG_RUNTIME_DIR/nerv.sock,
// falling back to $TMPDIR/nerv.sock when XDG_RUNTIME_DIR is unset.
func DefaultSocketPath() string {
	if sock := os.Getenv(EnvSocket); sock != "" {
		return sock
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = os.TempDir()
	}
	return filepath.Join(runtimeDir, "nerv.sock")
}

// udsHTTPClient returns an *http.Client whose Transport dials the given
// Unix domain socket path. ConnectRPC's default protocol runs over
// HTTP/1.1 so no h2c-specific transport is needed on the client side;
// the daemon also serves cleartext HTTP/2 (http.Protocols) for gRPC/h2c
// clients.
//
// The host portion of the URL passed to ConnectRPC ("http://unix") is
// ignored by this transport — DialContext always dials sockPath.
func udsHTTPClient(sockPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sockPath)
			},
		},
	}
}
