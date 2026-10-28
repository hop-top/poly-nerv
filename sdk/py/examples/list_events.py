"""Query a running nerv daemon for its event catalog.

Usage::

    nerv daemon &                                  # start the daemon
    python examples/list_events.py                 # default_socket_path()
    python examples/list_events.py /run/nerv.sock  # explicit socket path

Prints one line per event the daemon knows about.
"""

from __future__ import annotations

import sys

from hop_top_nerv import Client, default_socket_path


def main(socket_path: str) -> int:
    with Client(socket_path) as client:
        for ev in client.list():
            flags = "/".join(
                [
                    "blocking" if ev.blocking else "non-blocking",
                    ev.origin or "unknown",
                    ev.category or "uncategorized",
                ]
            )
            print(f"{ev.name:<28} {flags}")
    return 0


if __name__ == "__main__":
    if len(sys.argv) > 2:
        print("usage: list_events.py [socket_path]", file=sys.stderr)
        sys.exit(2)
    sock = sys.argv[1] if len(sys.argv) == 2 else default_socket_path()
    sys.exit(main(sock))
