# hop-top-nerv

Rust client SDK for **nerv** — the Universal Hooks Protocol daemon.

This crate connects to a running `nerv daemon` over its gRPC/UDS
surface and lets you dispatch envelopes or fetch the event catalog
from Rust applications.

> Status: alpha. API may change before `0.1.0` stable.

## Install

```toml
[dependencies]
hop-top-nerv = "0.1.0-alpha.0"
tokio = { version = "1", features = ["full"] }
```

Requires Rust 1.88 or newer (`rust-version` in `Cargo.toml`).
A working `protoc` (>= 3.21) is required on the build machine: the
proto schema is compiled by `tonic-prost-build` in `build.rs`.

## Quickstart

```rust
use hop_top_nerv::nerv_v1::{CliInfo, Envelope, SessionInfo};
use hop_top_nerv::{Client, default_socket_path};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut client = Client::connect_uds(default_socket_path()).await?;

    let events = client.list().await?;
    println!("{} events", events.len());

    let env = Envelope {
        nerv_version: "1.0.0-draft".into(),
        event: "PreToolUse".into(),
        event_id: "00000000-0000-0000-0000-000000000000".into(),
        cli: Some(CliInfo { name: "my-cli".into(), ..Default::default() }),
        session: Some(SessionInfo { id: "s1".into(), ..Default::default() }),
        ..Default::default()
    };
    let decision = client.dispatch(env).await?;
    println!("action: {}", decision.action);
    client.close();
    Ok(())
}
```

See [`examples/dispatch.rs`](https://github.com/hop-top/poly-nerv/blob/main/sdk/rs/examples/dispatch.rs) +
[`examples/list_events.rs`](https://github.com/hop-top/poly-nerv/blob/main/sdk/rs/examples/list_events.rs) for runnable
versions; [`examples/README.md`](https://github.com/hop-top/poly-nerv/blob/main/sdk/rs/examples/README.md) indexes them.

## Socket discovery

`default_socket_path()` resolves in this order, matching the Go SDK:

1. `$NERV_SOCKET` if set
2. `$XDG_RUNTIME_DIR/nerv.sock`
3. `env::temp_dir().join("nerv.sock")` (fallback)

## API surface

| Symbol | Purpose |
|--------|---------|
| `Client` | Async dispatcher over a configured tonic transport. |
| `Client::connect_uds(path)` | Connect to a Unix domain socket. |
| `Client::connect_tcp(uri)` | Connect to a TCP h2c endpoint (test convenience). |
| `Client::from_channel(channel)` | Wrap a pre-built `tonic::transport::Channel`. |
| `Client::dispatch(env) -> Decision` | Send one envelope, get the merged decision. |
| `Client::list() -> Vec<EventDescriptor>` | Fetch the daemon's event catalog. |
| `Client::close()` | Release the underlying channel. Mirrors Go's `Close()`. |
| `default_socket_path() -> PathBuf` | Resolve the conventional socket path. |
| `ENV_SOCKET` | Name of the env var consulted by `default_socket_path` (`NERV_SOCKET`). |
| `nerv_v1::*` | Protobuf-generated types (`Envelope`, `Decision`, …). |

## Build

```
cargo build
cargo test
cargo build --examples
```

## License

MIT. See [LICENSE](https://github.com/hop-top/poly-nerv/blob/main/LICENSE).
