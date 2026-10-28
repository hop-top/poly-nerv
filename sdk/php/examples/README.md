# hop-top/nerv examples

Runnable PHP scripts demonstrating the public SDK surface.

| Example | Purpose |
|---------|---------|
| `dispatch.php` | Dispatch a synthetic envelope through the SDK; prints the resolved socket path plus the decision. |
| `list_events.php` | Iterate the SDK's event-catalog surface; prints one line per known descriptor. |

Both examples run against the in-process `EventServiceStub`, so they
need neither a daemon nor the PECL `grpc` extension. To reach a running
daemon, replace the stub client with
`Client::fromSocket(Client::defaultSocketPath())`, which uses the real
gRPC wire transport and requires PECL `grpc`.

The socket path printed at startup comes from
`HopTop\Nerv\Client::defaultSocketPath()` (override with
`$NERV_SOCKET`); it is where `Client::fromSocket()` dials.

Run from `sdk/php/`:

```bash
composer install
php examples/dispatch.php
php examples/list_events.php
NERV_SOCKET=/tmp/nerv.sock php examples/dispatch.php
```
