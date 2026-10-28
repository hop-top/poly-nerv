# @hop-top/nerv

TypeScript SDK for **nerv** — the Universal Hooks Protocol daemon.

Two surfaces:

1. **Daemon client** (`NervClient`) — Node.js / Bun / Deno apps dispatch
   envelopes at a running nerv daemon over its Unix socket (or HTTP).
2. **Handler runtime** (`createHandler`) — write a nerv hook handler as a
   tiny stdin→stdout script.

## Install

```bash
npm install @hop-top/nerv
```

Requires Node.js >= 22.12.

## Daemon client

```typescript
import { create } from "@bufbuild/protobuf";
import {
  NervClient,
  EnvelopeSchema,
  DispatchRequestSchema,
  Action,
  defaultSocketPath,
} from "@hop-top/nerv";

const client = new NervClient({ socket: defaultSocketPath() });

try {
  const envelope = create(EnvelopeSchema, {
    nervVersion: "1.0.0-draft",
    event: "PreToolUse",
    eventId: crypto.randomUUID(),
    payload: { tool_name: "Bash", tool_input: { command: "echo hi" } },
  });

  const res = await client.events.dispatch(
    create(DispatchRequestSchema, { envelope }),
  );
  console.log(Action[res.decision!.action], res.decision!.message);

  const catalog = await client.events.list({});
  console.log(catalog.events.map((e) => e.name));
} finally {
  client.close();
}
```

### Transport

`NervClient` takes either a UDS path or an HTTP base URL:

```typescript
new NervClient({ socket: "/tmp/nerv.sock" });
new NervClient({ baseUrl: "http://127.0.0.1:7777" });
```

UDS uses HTTP/1.1 (the daemon's h2c listener accepts both). h2 is not
required for client traffic.

### Socket discovery

`defaultSocketPath()` resolves in this order, matching the Go SDK and
the `nerv daemon`:

1. `$NERV_SOCKET` if set
2. `$XDG_RUNTIME_DIR/nerv.sock`
3. `$TMPDIR/nerv.sock` (fallback)

## Handler runtime

```typescript
import { createHandler, events } from "@hop-top/nerv";

createHandler((envelope) => {
  const payload = envelope.payload as {
    tool_name?: string;
    tool_input?: { command?: string };
  };
  if (
    envelope.event === events.PreToolUse &&
    payload.tool_name === "Bash" &&
    String(payload.tool_input?.command ?? "").includes("rm -rf /")
  ) {
    return { action: "block", message: "Dangerous command blocked." };
  }
  return { action: "allow" };
});
```

The handler reads a JSON envelope from stdin, calls your function,
writes the decision to stdout, and exits with the matching exit code
(0=allow, 1=warn, 2=block, 3=rewrite).

## API

### Daemon client
- `NervClient(cfg)` — construct with `{ socket }` or `{ baseUrl }`
- `client.events.dispatch(req)` — fan out an envelope to handlers
- `client.events.list(req)` — fetch the static event catalog
- `client.close()` — release idle UDS keep-alive sockets; idempotent;
  no-op on `baseUrl` clients
- `defaultSocketPath()` — conventional UDS path lookup
- `ENV_SOCKET` — name of the env var consulted by `defaultSocketPath`
  (`"NERV_SOCKET"`)
- Re-exports: `Envelope`, `Decision`, `EventDescriptor`, `Action`,
  `EnvelopeSchema`, `DispatchRequestSchema`, etc. (Connect-ES v2 types)

### Handler runtime
- `createHandler(fn)` — register a handler function
- `events` — all nerv event name constants
- `HandlerEnvelope` / `HandlerDecision` — JSON-shape types for the stdin/stdout protocol
- `Event` — union type of all event names

See [`examples/dispatch.ts`](examples/dispatch.ts) and
[`examples/list-events.ts`](examples/list-events.ts) for runnable
programs.

## Status

Alpha (`0.1.0-alpha.0`). Wire types are stable per `contracts/proto/nerv/v1/event.proto`.

## License

MIT. See [LICENSE](https://github.com/hop-top/poly-nerv/blob/main/LICENSE).
