"""Dispatch a synthetic envelope to a running nerv daemon.

Usage::

    nerv daemon &                              # start the daemon
    python examples/dispatch.py                # uses default_socket_path()
    python examples/dispatch.py /run/nerv.sock # explicit socket path

Prints the merged Decision.action returned by the server.
"""

from __future__ import annotations

import sys
import time

from google.protobuf import struct_pb2, timestamp_pb2

from hop_top_nerv import Client, default_socket_path
from hop_top_nerv.client import event_pb2


def main(socket_path: str) -> int:
    ts = timestamp_pb2.Timestamp()
    ts.GetCurrentTime()

    payload = struct_pb2.Struct()
    payload.update({"tool_name": "Bash", "tool_input": {"command": "ls"}})

    envelope = event_pb2.Envelope(
        nerv_version="1.0.0-draft",
        event="PreToolUse",
        event_id=f"example-{int(time.time())}",
        timestamp=ts,
        cli=event_pb2.CLIInfo(name="examples/dispatch.py", version="0.1.0"),
        session=event_pb2.SessionInfo(id="example-session", cwd="/tmp"),
        payload=payload,
    )

    with Client(socket_path) as client:
        decision = client.dispatch(envelope)
        action_name = event_pb2.Action.Name(decision.action)
        print(f"decision: {action_name}", file=sys.stderr)
        if decision.message:
            print(f"message: {decision.message}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    if len(sys.argv) > 2:
        print("usage: dispatch.py [socket_path]", file=sys.stderr)
        sys.exit(2)
    sock = sys.argv[1] if len(sys.argv) == 2 else default_socket_path()
    sys.exit(main(sock))
