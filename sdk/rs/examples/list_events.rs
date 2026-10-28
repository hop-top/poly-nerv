//! Query a running `nerv daemon` for its event catalog and print one
//! line per known event.
//!
//! Run:
//!
//! ```text
//! cargo run --example list_events                  # default_socket_path()
//! cargo run --example list_events -- /tmp/nerv.sock
//! ```

use std::env;
use std::path::PathBuf;
use std::process;

use hop_top_nerv::{Client, default_socket_path};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = env::args().collect();
    let socket: PathBuf = match args.len() {
        1 => default_socket_path(),
        2 => PathBuf::from(&args[1]),
        _ => {
            eprintln!("usage: list_events [socket_path]");
            process::exit(2);
        }
    };

    let mut client = Client::connect_uds(&socket).await?;
    let events = client.list().await?;
    for ev in &events {
        let flags = [
            if ev.blocking {
                "blocking"
            } else {
                "non-blocking"
            },
            if ev.origin.is_empty() {
                "unknown"
            } else {
                ev.origin.as_str()
            },
            if ev.category.is_empty() {
                "uncategorized"
            } else {
                ev.category.as_str()
            },
        ]
        .join(" / ");
        println!("{:<28} {}", ev.name, flags);
    }
    Ok(())
}
