# nerv Go SDK

Client SDK for dispatching hook envelopes to a running [nerv](../../)
daemon over its Unix domain socket transport (ConnectRPC).

## Status

Alpha. Ships as its own Go module (`hop.top/nerv/sdk/go`) separate
from the daemon module (`hop.top/nerv`). It requires `hop.top/nerv`
at the same release for the generated protobuf stubs
(`hop.top/nerv/gen/go/...`), so the root module's requirements join
your module graph. Only the packages the SDK imports are built:
the SDK, the generated stubs, `connectrpc.com/connect`, and
`google.golang.org/protobuf`.

The module is published as a nested module on the `hop-top/nerv`
mirror, tagged `sdk/go/vX.Y.Z` (monorepo tag `nerv-go/vX.Y.Z`). Linked to
the polyglot `nerv` group in release-please so SDK versions track
the contract.

## Install

```bash
go get hop.top/nerv/sdk/go
```

Available after the first tagged release. Requires Go 1.27.3 or
newer (the `go` directive in `go.mod`).

## Usage

```go
package main

import (
    "context"
    "log"

    nervsdk "hop.top/nerv/sdk/go"
    nervv1 "hop.top/nerv/gen/go/nerv/v1"
)

func main() {
    client := nervsdk.NewUDS(nervsdk.DefaultSocketPath())
    defer func() {
        if err := client.Close(); err != nil {
            log.Printf("close: %v", err)
        }
    }()

    decision, err := client.Dispatch(context.Background(), &nervv1.Envelope{
        NervVersion: "1.0.0-draft",
        Event:       "PreToolUse",
        EventId:     "00000000-0000-0000-0000-000000000001",
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("decision: %s — %s", decision.GetAction(), decision.GetMessage())
}
```

See [`examples/dispatch`](examples/dispatch/main.go) for a runnable
program.

## Socket discovery

`DefaultSocketPath()` resolves in this order:

1. `$NERV_SOCKET` if set
2. `$XDG_RUNTIME_DIR/nerv.sock`
3. `$TMPDIR/nerv.sock` (fallback)

This matches the nerv daemon (`cmd/nervd`, `nerv daemon`).

## API surface

| Symbol | Purpose |
|--------|---------|
| `Client` | Concurrency-safe dispatcher over a configured transport. |
| `NewUDS(sockPath)` | Construct a Client bound to a Unix domain socket. |
| `NewWithHTTPClient(http, baseURL)` | Construct over an arbitrary `*http.Client` (TCP, httptest, custom dialer). |
| `(*Client).Dispatch(ctx, env)` | Send one envelope, get the merged decision. |
| `(*Client).List(ctx)` | Fetch the daemon's event catalog. |
| `(*Client).Close()` | Release idle connections. Idempotent. |
| `DefaultSocketPath()` | Resolve the conventional socket path. |
| `EnvSocket` | Name of the env var consulted by `DefaultSocketPath` (`NERV_SOCKET`). |

## Transport notes

The UDS transport is a stdlib `*http.Transport` with a custom
`DialContext`; ConnectRPC speaks HTTP/1.1 by default so no `h2c`
client wrapper is needed. The daemon also serves cleartext HTTP/2
(`http.Server.Protocols`) for gRPC/h2c clients.

## License

MIT. See [LICENSE](https://github.com/hop-top/poly-nerv/blob/main/LICENSE).
