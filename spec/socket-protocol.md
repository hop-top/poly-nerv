# Nerv Daemon Socket Protocol

> Opt-in daemon mode only. Stateless mode (default) does not use
> sockets — it forks nervd per event via stdin/stdout.

## Transport

- **Socket:** Unix domain socket at `$XDG_RUNTIME_DIR/nerv.sock`
  (resolved via `kit/xdg`; darwin fallback to `$TMPDIR/nerv.sock`)
- **Ownership:** socket created by `nerv daemon start`; removed on
  clean shutdown
- **Permissions:** `0600` — owner-only access

## Wire Format

Length-prefixed JSON:

```
[4 bytes: big-endian uint32 length][JSON payload of exactly that length]
```

- One message per envelope; one response per message.
- Max message size: 10 MiB. Messages exceeding limit rejected with
  `PAYLOAD_TOO_LARGE` error.
- Encoding: UTF-8 JSON. No compression. No binary framing beyond
  the length prefix.

## Session Lifecycle

### Handshake

```
Client                              Server
  |                                    |
  |--- connect (unix socket) --------->|
  |                                    |
  |--- hello ------------------------->|
  |                                    |
  |<-- hello_ack ---------------------|
  |                                    |
  |    (session established)           |
```

**Client sends:**

```json
{
  "type": "hello",
  "protocol_version": "1.0",
  "cli": {
    "name": "claude",
    "version": "1.2.3"
  },
  "session_id": "sess_abc123"
}
```

**Server responds:**

```json
{
  "type": "hello_ack",
  "nerv_version": "0.1.0",
  "capabilities": [
    "redaction",
    "decision_merge",
    "event_log"
  ]
}
```

Capabilities list informs the client which features are active.
Client MAY adjust behavior based on capabilities (e.g., skip
client-side redaction if server reports `redaction`).

### Steady State

After handshake, client sends envelopes; server replies with
decisions.

```
Client                              Server
  |                                    |
  |--- envelope ---------------------->|
  |                                    |
  |<-- decision ----------------------|
  |                                    |
  |--- heartbeat --------------------->|  (independent; no reply expected)
  |                                    |
  |                  heartbeat ------->|  (server sends independently too)
  |                                    |
```

## Message Types

### envelope (client -> server)

Carries a nerv envelope for dispatch.

```json
{
  "type": "envelope",
  "id": "evt_001",
  "envelope": {
    "event": "PreToolUse",
    "cli": {"name": "claude", "version": "1.2.3"},
    "session_id": "sess_abc123",
    "timestamp": "2026-04-11T12:00:00Z",
    "tool_name": "Bash",
    "tool_input": {"command": "ls -la"},
    "synthesized": false
  }
}
```

### decision (server -> client)

Carries the merged decision after handler dispatch.

```json
{
  "type": "decision",
  "id": "evt_001",
  "decision": {
    "action": "allow",
    "warnings": [],
    "reason": null,
    "rewrite": null
  }
}
```

Decision `action` values: `allow`, `warn`, `block`, `rewrite`.

The `id` field matches the originating envelope, allowing the
client to correlate responses to requests.

### heartbeat (bidirectional)

Sent every 30 seconds by both sides. Missing 3 consecutive
heartbeats triggers connection close.

```json
{
  "type": "heartbeat",
  "ts": "2026-04-11T12:00:30Z"
}
```

### goodbye (client -> server)

Graceful disconnect initiated by client.

```json
{
  "type": "goodbye",
  "reason": "session_end"
}
```

Server acknowledges by closing the connection after draining
in-flight decisions.

### shutdown (server -> client)

Server-initiated graceful shutdown.

```json
{
  "type": "shutdown",
  "reason": "daemon_restart"
}
```

Protocol after shutdown:

1. Server sends `shutdown` to all connected clients.
2. Clients have **5 seconds** to drain in-flight envelopes.
3. Server waits for all in-flight decisions to complete.
4. Server closes all connections and removes socket file.

### error (server -> client)

Sent on protocol violations or server issues.

```json
{
  "type": "error",
  "code": "PARSE_ERROR",
  "message": "invalid JSON at byte 42"
}
```

## Error Codes

| Code                | Trigger                          | Client action            |
|---------------------|----------------------------------|--------------------------|
| `PARSE_ERROR`       | Malformed message                | Server closes connection |
| `UNKNOWN_TYPE`      | Unrecognized message type        | Server closes connection |
| `PAYLOAD_TOO_LARGE` | Message > 10 MiB                 | Server closes connection |
| `OVERLOADED`        | Server at capacity               | Fall back to stateless   |
| `INTERNAL`          | Unexpected server error          | Retry or fall back       |
| `HANDSHAKE_TIMEOUT` | No hello within 5s of connect    | Server closes connection |
| `VERSION_MISMATCH`  | Incompatible protocol version    | Server closes connection |

## Connection Limits

- Max concurrent connections: **64** (one per CLI session)
- Max in-flight envelopes per connection: **16**
- Handshake timeout: **5 seconds**
- Heartbeat interval: **30 seconds**
- Heartbeat miss tolerance: **3** (90s total before disconnect)

## Fallback

When daemon is unavailable or returns `OVERLOADED`:

1. Client adapter detects socket unreachable or error response.
2. Falls back to stateless mode: fork nervd, pipe envelope via
   stdin, read decision from stdout.
3. Logs fallback event for diagnostics (`nerv doctor` reports it).
4. Retries daemon connection on next event (no backoff; daemon
   either recovers or stays down).

## Concurrency

- Server dispatches envelopes concurrently per connection.
- Decisions returned out-of-order; client correlates via `id`.
- Handler execution is parallel across handlers for a single
  envelope; decision merge waits for all handlers.
- Ordering guarantee: none. Envelopes from the same CLI session
  may interleave. Handlers must be stateless or manage their own
  synchronization.

## Security

- Socket file permissions `0600` — only the owning user can
  connect.
- No authentication beyond filesystem permissions (same-user
  trust model).
- No TLS — unix socket is local-only.
- Envelope content subject to redaction before dispatch (same
  pipeline as stateless mode).
