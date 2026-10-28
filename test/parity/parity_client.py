"""Parity client for the Python SDK.

Reads NERV_SOCKET from env, dials the daemon, calls List then Dispatch
against the canonical parity contract, and prints both markers on stdout
(list event name + dispatch decision message). The Go harness
(TestParity_Python) asserts stdout contains both.

Invoked as: NERV_SOCKET=/tmp/n.sock sdk/py/.venv/bin/python3 \
              test/parity/parity_client.py
"""

from __future__ import annotations

import os
import sys

from hop_top_nerv import Client
from hop_top_nerv.client import event_pb2


def main() -> int:
    socket = os.environ.get("NERV_SOCKET")
    if not socket:
        print("NERV_SOCKET unset", file=sys.stderr)
        return 2

    envelope = event_pb2.Envelope(
        nerv_version="1.0.0-draft",
        event="PreToolUse",
        event_id="parity-event",
        cli=event_pb2.CLIInfo(name="claude", version="0.0.0"),
        session=event_pb2.SessionInfo(id="parity-session", cwd="/tmp"),
    )

    with Client(socket) as client:
        events = client.list()
        first_name = events[0].name if events else ""
        sys.stdout.write(first_name + "\n")

        decision = client.dispatch(envelope)
        sys.stdout.write((decision.message or "") + "\n")

    return 0


if __name__ == "__main__":
    sys.exit(main())
