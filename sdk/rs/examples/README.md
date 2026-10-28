# hop-top-nerv examples

Runnable Rust programs demonstrating the public SDK surface.

| Example | Purpose |
|---------|---------|
| `dispatch.rs` | Send a synthetic envelope at a running `nerv daemon` and print the merged decision. |
| `list_events.rs` | Query the daemon's event catalog and print one line per known event. |

Each example resolves the socket path via
`hop_top_nerv::default_socket_path()` (override with `$NERV_SOCKET`)
unless an explicit path is passed as the first CLI argument.

Run from `sdk/rs/`:

```bash
cargo run --example dispatch
cargo run --example list_events
cargo run --example list_events -- /tmp/nerv.sock
```

Both examples require a running `nerv daemon` reachable at the
resolved socket path. Start one from the repo root with:

```bash
go run ./cmd/nerv daemon start
# or, in the foreground:
NERV_SOCKET=/tmp/nerv.sock go run ./cmd/nervd
```
