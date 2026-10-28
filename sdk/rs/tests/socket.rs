//! Tests for [`default_socket_path`] + [`ENV_SOCKET`].
//!
//! Cargo runs `#[test]` functions within a single test binary in
//! parallel by default; these mutate process-wide env vars, so a
//! Mutex guards execution. Mirrors the Go SDK's
//! `TestDefaultSocketPath_*` cases.

use std::env;
use std::path::PathBuf;
use std::sync::Mutex;

use hop_top_nerv::{ENV_SOCKET, default_socket_path};

static ENV_LOCK: Mutex<()> = Mutex::new(());

/// RAII guard that records + restores an env var's prior value.
struct EnvGuard {
    key: &'static str,
    prior: Option<String>,
}

impl EnvGuard {
    fn set(key: &'static str, value: &str) -> Self {
        let prior = env::var(key).ok();
        // SAFETY: tests serialize all env mutation via ENV_LOCK below.
        unsafe { env::set_var(key, value) };
        Self { key, prior }
    }

    fn unset(key: &'static str) -> Self {
        let prior = env::var(key).ok();
        // SAFETY: tests serialize all env mutation via ENV_LOCK below.
        unsafe { env::remove_var(key) };
        Self { key, prior }
    }
}

impl Drop for EnvGuard {
    fn drop(&mut self) {
        // SAFETY: tests serialize all env mutation via ENV_LOCK below.
        unsafe {
            match &self.prior {
                Some(v) => env::set_var(self.key, v),
                None => env::remove_var(self.key),
            }
        }
    }
}

#[test]
fn env_socket_constant_matches_daemon_convention() {
    assert_eq!(ENV_SOCKET, "NERV_SOCKET");
}

#[test]
fn default_socket_path_respects_env() {
    let _lock = ENV_LOCK.lock().unwrap_or_else(|e| e.into_inner());
    let _g1 = EnvGuard::set(ENV_SOCKET, "/custom/path.sock");
    let _g2 = EnvGuard::set("XDG_RUNTIME_DIR", "/run/user/1000");
    assert_eq!(default_socket_path(), PathBuf::from("/custom/path.sock"));
}

#[test]
fn default_socket_path_xdg_fallback() {
    let _lock = ENV_LOCK.lock().unwrap_or_else(|e| e.into_inner());
    let _g1 = EnvGuard::unset(ENV_SOCKET);
    let _g2 = EnvGuard::set("XDG_RUNTIME_DIR", "/run/user/1000");
    assert_eq!(
        default_socket_path(),
        PathBuf::from("/run/user/1000/nerv.sock")
    );
}

#[test]
fn default_socket_path_tmp_fallback_when_xdg_unset() {
    let _lock = ENV_LOCK.lock().unwrap_or_else(|e| e.into_inner());
    let _g1 = EnvGuard::unset(ENV_SOCKET);
    let _g2 = EnvGuard::unset("XDG_RUNTIME_DIR");
    let expected = env::temp_dir().join("nerv.sock");
    assert_eq!(default_socket_path(), expected);
}

#[test]
fn default_socket_path_treats_empty_env_socket_as_unset() {
    let _lock = ENV_LOCK.lock().unwrap_or_else(|e| e.into_inner());
    let _g1 = EnvGuard::set(ENV_SOCKET, "");
    let _g2 = EnvGuard::set("XDG_RUNTIME_DIR", "/run/user/1000");
    assert_eq!(
        default_socket_path(),
        PathBuf::from("/run/user/1000/nerv.sock")
    );
}
