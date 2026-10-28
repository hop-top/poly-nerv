//! hop-top-nerv — Rust client SDK for the nerv Universal Hooks Protocol.
//!
//! Connects to a running `nerv daemon` over a Unix domain socket (or
//! TCP for testing) and dispatches event envelopes via gRPC.
//!
//! ```no_run
//! use hop_top_nerv::Client;
//!
//! # async fn run() -> Result<(), Box<dyn std::error::Error>> {
//! let mut client = Client::connect_uds("/tmp/nerv.sock").await?;
//! let events = client.list().await?;
//! println!("{} events known", events.len());
//! # Ok(())
//! # }
//! ```
//!
//! Generated protobuf types live in [`nerv_v1`].

#![doc(html_root_url = "https://docs.rs/hop-top-nerv/0.1.0-alpha.0")]

mod client;
mod error;
mod socket;
mod transport;

pub use client::Client;
pub use error::{ConnectError, DispatchError};
pub use socket::{ENV_SOCKET, default_socket_path};

/// Generated protobuf types for `nerv.v1`.
// tonic-build output; clippy findings there are not ours to fix.
#[allow(clippy::all)]
pub mod nerv_v1 {
    tonic::include_proto!("nerv.v1");
}
