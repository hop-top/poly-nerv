# Cross-SDK parity harness

Wire-compatibility smoke for every nerv SDK. Spawns an in-process Go
daemon over a UDS, binds a fake `EventService` with deterministic
markers, and drives each SDK against that socket.

## What it asserts

Full SDK surface = **List + Dispatch**. Per language:

| SDK | How it runs | Asserts |
|-----|-------------|---------|
| Go | in-process via `sdk/go` | `list()[0].Name == "PreToolUse"` + `decision.Message == "parity-marker"` |
| TypeScript | shells out to `node parity_client.mjs` | stdout contains `"PreToolUse"` + `"parity-marker"` |
| Python | shells out to `.venv/bin/python3 parity_client.py` | stdout contains both |
| Rust | shells out to pre-built `target/debug/examples/parity_client` | stdout contains both |
| PHP | shells out to `php parity_client.php` | stdout contains both (skips when wire unavailable) |

`Close` is **not** asserted here. It is a client-side cleanup concern
verified by each SDK's own `*Client_Close_Idempotent` test.

## Run locally

All five tests:

```bash
cd hops/main
/usr/bin/env go test -count=1 -v ./test/parity/...
```

A test skips when its toolchain or build artifact is missing. Concrete
preconditions per language:

| Test | Skip cause | Fix |
|------|-----------|-----|
| `TestParity_Go` | — | n/a (always runs) |
| `TestParity_TypeScript` | `node` missing | install Node >= 22.12 (CI runs 24) |
| `TestParity_TypeScript` | `sdk/ts/dist/index.js` missing | `cd sdk/ts && pnpm install && pnpm run build` |
| `TestParity_TypeScript` | `sdk/ts/node_modules` missing | `cd sdk/ts && pnpm install` |
| `TestParity_Python` | `sdk/py/.venv/bin/python3` missing | `python3.12 -m venv sdk/py/.venv && sdk/py/.venv/bin/pip install -e sdk/py` |
| `TestParity_Rust` | `cargo` missing | install Rust >= 1.88 (`rust-version` in `sdk/rs/Cargo.toml`) |
| `TestParity_Rust` | example build fails: `Could not find protoc` | install `protoc` (>= 3.21) or set `PROTOC` |
| `TestParity_Rust` | first run | harness pre-builds the example once via `sync.Once`; subsequent runs reuse `sdk/rs/target/debug/examples/parity_client` |
| `TestParity_PHP` | `php` missing | install PHP 8.2+ |
| `TestParity_PHP` | `sdk/php/vendor/` missing AND `composer` missing | install Composer |
| `TestParity_PHP` | PECL `grpc` extension not loaded (script exits 3) | `pecl install grpc` (or `apt install php-grpc`) |

## When a parity test fails

A failure means an SDK's wire encoding diverged from the proto contract
the daemon expects. Triage path:

1. Read the test output. The harness prints the full stdout/stderr from
   the failing client.
2. Reproduce the client manually. Each `parity_client.<ext>` reads
   `NERV_SOCKET` from env — start the daemon (or any UDS test fixture),
   then invoke the client directly. Example for Rust:
   ```bash
   NERV_SOCKET=/tmp/n.sock go run ./cmd/nervd &
   NERV_SOCKET=/tmp/n.sock sdk/rs/target/debug/examples/parity_client
   ```
3. Compare wire-level traffic. The contract is in
   `contracts/proto/nerv/v1/event.proto`. If the SDK's generated stubs
   are out of date, regenerate them (per-SDK `buf generate` / cargo
   `build.rs` / etc.).
4. Per-SDK unit tests live next to each SDK (`sdk/<lang>/`). The parity
   harness only catches *cross*-SDK drift; in-language correctness
   stays a per-SDK concern.

## Adding a new SDK

1. Add the SDK to `sdk/<lang>/`.
2. Write `parity_client.<ext>` next to the existing drivers. Shape:
   read `NERV_SOCKET`, call `list()`, print the first event name, call
   `dispatch(envelope)`, print the decision message.
3. Add `TestParity_<Lang>` to `parity_test.go` mirroring the closest
   existing test (compiled-binary vs. interpreter-script).
4. Update the table above + this README's "Run locally" section.
5. Update `.github/workflows/ci.yml` `jobs.go` to install the
   toolchain before `go test` runs.

## Why this exists

A change in one SDK's protobuf wire emission would silently break the
others until production traffic surfaced it. This harness catches that
drift at PR time. Five language tests, one in-process daemon, one
canonical envelope — wire-level parity by construction.
