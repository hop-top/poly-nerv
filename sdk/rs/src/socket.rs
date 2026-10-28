//! Default socket discovery — mirrors the Go SDK's
//! [`DefaultSocketPath`] + [`EnvSocket`] surface.

use std::env;
use std::path::PathBuf;

/// Name of the environment variable consulted by
/// [`default_socket_path`] to override the XDG-derived default.
///
/// Matches the nerv daemon convention used by `cmd/nervd` and
/// `nerv daemon`.
pub const ENV_SOCKET: &str = "NERV_SOCKET";

/// Return the conventional UDS path for a local nerv daemon.
///
/// Resolution order (matches the Go SDK and the nerv daemon):
///
/// 1. `$NERV_SOCKET` if set and non-empty
/// 2. `$XDG_RUNTIME_DIR/nerv.sock`
/// 3. `$TMPDIR/nerv.sock` (fallback via [`std::env::temp_dir`])
pub fn default_socket_path() -> PathBuf {
    if let Ok(sock) = env::var(ENV_SOCKET)
        && !sock.is_empty()
    {
        return PathBuf::from(sock);
    }
    let runtime_dir = env::var("XDG_RUNTIME_DIR")
        .ok()
        .filter(|s| !s.is_empty())
        .map(PathBuf::from)
        .unwrap_or_else(env::temp_dir);
    runtime_dir.join("nerv.sock")
}
