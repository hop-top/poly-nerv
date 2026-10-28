# nerv-sdk

Python SDK for nerv — Universal Hooks Protocol.

PyPI name: ``nerv-sdk``. Import path: ``hop_top_nerv``.

Two surfaces:

* ``Client`` — dispatch envelopes to a running ``nerv daemon`` over its
  Unix domain socket (gRPC over h2c).
* ``create_handler`` — stdin/stdout handler contract for processes the
  daemon invokes (see ``spec/handler-contract.md``).

## Install

```bash
pip install nerv-sdk
```

Requires Python 3.11 or newer.

## Status

Alpha (`0.1.0-alpha.0`). Wire types follow
`contracts/proto/nerv/v1/event.proto`; the API may change before 1.0.

## Dispatch from a client app

```python
from google.protobuf import struct_pb2, timestamp_pb2

from hop_top_nerv import Client, default_socket_path
from hop_top_nerv.client import event_pb2

ts = timestamp_pb2.Timestamp(); ts.GetCurrentTime()
payload = struct_pb2.Struct(); payload.update({"tool_name": "Bash"})

envelope = event_pb2.Envelope(
    nerv_version="1.0.0-draft",
    event="PreToolUse",
    event_id="my-id",
    timestamp=ts,
    cli=event_pb2.CLIInfo(name="my-cli", version="0.1.0"),
    session=event_pb2.SessionInfo(id="sess-1"),
    payload=payload,
)

with Client(default_socket_path()) as c:
    decision = c.dispatch(envelope)
    print(event_pb2.Action.Name(decision.action))
    for ev in c.list():
        print(ev.name, ev.category)
```

### Socket discovery

``default_socket_path()`` resolves in this order, matching the Go SDK
and the ``nerv daemon``:

1. ``$NERV_SOCKET`` if set
2. ``$XDG_RUNTIME_DIR/nerv.sock``
3. system tempdir + ``nerv.sock`` (fallback)

For tests or non-UDS transports, build a Client around an existing
``grpc.Channel`` via ``Client.from_channel(channel)`` (the Python
analogue of the Go SDK's ``NewWithHTTPClient``).

## Register a handler

```python
from hop_top_nerv import Decision, Envelope, create_handler
from hop_top_nerv.events import PRE_TOOL_USE

def handler(envelope: Envelope) -> Decision:
    if envelope.event == PRE_TOOL_USE:
        cmd = envelope.payload.get("tool_input", {}).get("command", "")
        if "rm -rf /" in cmd:
            return Decision(action="block", message="Dangerous command blocked.")
    return Decision(action="allow")

create_handler(handler)
```

The handler reads a JSON envelope from stdin, calls your function,
writes the decision to stdout, and exits with the correct code
(0=allow, 1=warn, 2=block, 3=rewrite).

## API

### Client (dispatcher)

- ``Client(socket_path, timeout=5.0)`` — open a gRPC channel on
  ``unix:<socket_path>``.
- ``Client.from_channel(channel, timeout=5.0)`` — wrap an arbitrary
  ``grpc.Channel`` (tests, TCP, custom dialers).
- ``Client.dispatch(envelope)`` — returns ``Decision`` (merged
  block > rewrite > warn > allow).
- ``Client.list()`` — returns ``list[EventDescriptor]``.
- ``Client.close()`` — release the channel. Idempotent. Also usable
  as a context manager.
- ``default_socket_path()`` — conventional UDS path lookup.
- ``ENV_SOCKET`` — name of the env var consulted by
  ``default_socket_path`` (``"NERV_SOCKET"``).

### Handler (stdin/stdout)

- ``create_handler(fn)`` — register your handler function.
- ``Envelope`` — dataclass for the input envelope.
- ``Decision`` — dataclass for the output decision.
- ``events()`` / ``EVENTS`` — all nerv event names.
- Individual constants: ``PRE_TOOL_USE``, ``SESSION_START``, etc.

See [`examples/dispatch.py`](https://github.com/hop-top/poly-nerv/blob/main/sdk/py/examples/dispatch.py) and
[`examples/list_events.py`](https://github.com/hop-top/poly-nerv/blob/main/sdk/py/examples/list_events.py) for runnable
programs.

## Development

```bash
python -m venv .venv && source .venv/bin/activate
pip install -e ".[dev]"
pytest tests/
```

Generated stubs under ``src/hop_top_nerv/gen/`` are produced by ``buf
generate`` (see ``buf.gen.yaml``); do not edit by hand.

## License

MIT. See [LICENSE](https://github.com/hop-top/poly-nerv/blob/main/LICENSE).
