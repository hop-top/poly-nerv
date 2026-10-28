"""nerv gRPC client over a Unix domain socket.

The nerv daemon hosts ConnectRPC's EventService via h2c on a UDS
(see internal/daemon/server.go). ConnectRPC accepts gRPC clients, so
this module uses ``grpcio`` with the ``unix:`` channel scheme. The
generated stubs live under ``hop_top_nerv.gen.nerv.v1``; importing
that package as a side effect adds ``gen/`` to ``sys.path`` so the
top-level ``nerv.v1`` imports inside ``event_pb2_grpc.py`` resolve.

Alpha. Synchronous unary methods only. Reuse the Client across calls
to amortise channel setup.
"""

from __future__ import annotations

import os
import tempfile
from typing import Self

import grpc

# Importing the gen package triggers the sys.path shim so the
# generated grpc module's top-level "from nerv.v1 import ..." works.
from hop_top_nerv import gen as _gen  # noqa: F401

# isort: split
from nerv.v1 import event_pb2, event_pb2_grpc

__all__ = [
    "ENV_SOCKET",
    "Client",
    "default_socket_path",
    "event_pb2",
    "event_pb2_grpc",
]


# Env var consulted by ``default_socket_path`` to override the
# XDG-derived default. Mirrors the Go SDK's ``EnvSocket`` constant.
ENV_SOCKET = "NERV_SOCKET"


def default_socket_path() -> str:
    """Resolve the conventional UDS path for a local nerv daemon.

    Resolution order matches the Go SDK and the nerv daemon
    (``cmd/nervd``, ``nerv daemon``):

    1. ``$NERV_SOCKET`` if set
    2. ``$XDG_RUNTIME_DIR/nerv.sock``
    3. ``$TMPDIR/nerv.sock`` (fallback)
    """
    override = os.environ.get(ENV_SOCKET)
    if override:
        return override
    runtime_dir = os.environ.get("XDG_RUNTIME_DIR") or tempfile.gettempdir()
    return os.path.join(runtime_dir, "nerv.sock")


class Client:
    """Synchronous nerv EventService client over a Unix socket.

    Args:
        socket_path: filesystem path of the daemon's UDS, e.g.
            ``/run/nerv.sock`` or ``~/.local/state/nerv/nerv.sock``.
        timeout: per-RPC deadline in seconds. ``None`` blocks
            indefinitely.

    Example:
        >>> client = Client("/run/nerv.sock")
        >>> decision = client.dispatch(envelope)
        >>> client.close()

    For tests or non-UDS transports, use :meth:`from_channel` to
    wrap an existing ``grpc.Channel`` (the Python analogue of the Go
    SDK's ``NewWithHTTPClient``).
    """

    def __init__(self, socket_path: str, *, timeout: float | None = 5.0) -> None:
        if not socket_path:
            raise ValueError("socket_path required")
        self._socket_path = socket_path
        self._timeout = timeout
        self._channel = grpc.insecure_channel(f"unix:{socket_path}")
        self._stub = event_pb2_grpc.EventServiceStub(self._channel)

    @classmethod
    def from_channel(
        cls,
        channel: grpc.Channel,
        *,
        timeout: float | None = 5.0,
    ) -> Client:
        """Build a Client around an arbitrary gRPC channel.

        Use this for tests (in-process fake channel), TCP transports,
        or anywhere a raw ``grpc.insecure_channel("unix:...")`` is
        the wrong shape. Mirrors the Go SDK's ``NewWithHTTPClient``.

        The caller retains channel ownership; ``close()`` on this
        Client still calls ``channel.close()`` for symmetry with the
        UDS constructor.
        """
        obj = cls.__new__(cls)
        obj._socket_path = ""
        obj._timeout = timeout
        obj._channel = channel
        obj._stub = event_pb2_grpc.EventServiceStub(channel)
        return obj

    @property
    def socket_path(self) -> str:
        """The UDS path this client is configured to dial.

        Empty string when the Client was built via
        :meth:`from_channel` (no socket-path-based identity).
        """
        return self._socket_path

    def dispatch(self, envelope: event_pb2.Envelope) -> event_pb2.Decision:
        """Send ``envelope`` to the daemon and return the merged decision.

        Decision precedence (server-side): block > rewrite > warn > allow.
        """
        request = event_pb2.DispatchRequest(envelope=envelope)
        response = self._stub.Dispatch(request, timeout=self._timeout)
        return response.decision

    def list(self) -> list[event_pb2.EventDescriptor]:
        """Return the daemon's static catalog of known events."""
        response = self._stub.List(event_pb2.ListRequest(), timeout=self._timeout)
        return list(response.events)

    def close(self) -> None:
        """Release the underlying gRPC channel.

        Idempotent; safe to call on a never-used Client. Mirrors the
        Go SDK's ``(*Client).Close()`` contract.
        """
        self._channel.close()

    def __enter__(self) -> Self:
        return self

    def __exit__(self, exc_type, exc, tb) -> None:
        self.close()
