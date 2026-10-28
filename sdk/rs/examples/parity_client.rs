//! Parity client for the Rust SDK.
//!
//! Reads `NERV_SOCKET` from env, dials the daemon, calls List then
//! Dispatch against the canonical parity contract, and prints both
//! markers on stdout (list event name + dispatch decision message).
//! The Go harness (`TestParity_Rust`) asserts stdout contains both.
//!
//! Invoked as:
//!
//! ```text
//! NERV_SOCKET=/tmp/n.sock target/debug/examples/parity_client
//! ```

use std::env;
use std::process;

use hop_top_nerv::Client;
use hop_top_nerv::nerv_v1::{CliInfo, Envelope, SessionInfo};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let socket = match env::var("NERV_SOCKET") {
        Ok(v) if !v.is_empty() => v,
        _ => {
            eprintln!("NERV_SOCKET unset");
            process::exit(2);
        }
    };

    let mut client = Client::connect_uds(&socket).await?;

    let events = client.list().await?;
    let first_name = events.first().map(|e| e.name.as_str()).unwrap_or("");
    println!("{}", first_name);

    let envelope = Envelope {
        nerv_version: "1.0.0-draft".into(),
        event: "PreToolUse".into(),
        event_id: "parity-event".into(),
        timestamp: None,
        cli: Some(CliInfo {
            name: "claude".into(),
            version: "0.0.0".into(),
            binary_path: String::new(),
        }),
        session: Some(SessionInfo {
            id: "parity-session".into(),
            project_key: String::new(),
            cwd: "/tmp".into(),
        }),
        payload: None,
        synthesized: false,
        synthesis_source: 0,
        capabilities: None,
    };

    let decision = client.dispatch(envelope).await?;
    println!("{}", decision.message);

    client.close();
    Ok(())
}
