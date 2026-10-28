"""nerv-sdk — Python SDK for the Universal Hooks Protocol.

Two complementary surfaces:

* ``Client`` — synchronous gRPC client that dispatches envelopes to a
  running ``nerv daemon`` over its Unix domain socket. Use this from
  CLIs and apps that want to forward hook events into nerv.
* ``create_handler`` + ``Envelope`` + ``Decision`` — the stdin/stdout
  handler contract for processes the daemon invokes per spec/handler-contract.md.
"""

from hop_top_nerv.client import ENV_SOCKET, Client, default_socket_path
from hop_top_nerv.events import EVENTS, events
from hop_top_nerv.handler import (
    CliInfo,
    Decision,
    Envelope,
    SessionInfo,
    create_handler,
)

__all__ = [
    "ENV_SOCKET",
    "EVENTS",
    "CliInfo",
    "Client",
    "Decision",
    "Envelope",
    "SessionInfo",
    "create_handler",
    "default_socket_path",
    "events",
]
