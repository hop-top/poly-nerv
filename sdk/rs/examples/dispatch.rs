//! Minimal runnable example: dispatch a synthetic envelope at a
//! running `nerv daemon`.
//!
//! Run with:
//!
//! ```text
//! cargo run --example dispatch                  # uses default_socket_path()
//! NERV_SOCKET=/tmp/nerv.sock cargo run --example dispatch
//! ```

use std::env;

use hop_top_nerv::Client;
use hop_top_nerv::default_socket_path;
use hop_top_nerv::nerv_v1::{CliInfo, Envelope, SessionInfo};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let socket = default_socket_path();
    let mut client = Client::connect_uds(&socket).await?;

    println!("listing events from {}...", socket.display());
    let events = client.list().await?;
    println!("daemon knows {} events", events.len());

    let envelope = Envelope {
        nerv_version: "1.0.0-draft".into(),
        event: "PreToolUse".into(),
        event_id: "00000000-0000-0000-0000-000000000000".into(),
        timestamp: None,
        cli: Some(CliInfo {
            name: "hop-top-nerv-example".into(),
            version: env!("CARGO_PKG_VERSION").into(),
            binary_path: env::current_exe()
                .ok()
                .and_then(|p| p.to_str().map(str::to_owned))
                .unwrap_or_default(),
        }),
        session: Some(SessionInfo {
            id: "example-session".into(),
            project_key: String::new(),
            cwd: env::current_dir()
                .ok()
                .and_then(|p| p.to_str().map(str::to_owned))
                .unwrap_or_default(),
        }),
        payload: None,
        synthesized: true,
        synthesis_source: 0,
        capabilities: None,
    };

    let decision = client.dispatch(envelope).await?;
    println!(
        "decision: action={} message={:?}",
        decision.action, decision.message
    );
    Ok(())
}
