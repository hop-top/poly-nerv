"""Tests for hop_top_nerv.Client.

Mirrors sdk/go/client_test.go: spins up an in-process gRPC server on
a real UDS, exercises Dispatch + List round-trips, error propagation,
Close idempotency, and the default_socket_path env+XDG fallback.
"""

from __future__ import annotations

import os
import tempfile
from collections.abc import Iterator
from concurrent import futures
from unittest.mock import MagicMock

import grpc
import pytest

from hop_top_nerv import (
    ENV_SOCKET,
    Client,
    default_socket_path,
)
from hop_top_nerv.client import event_pb2, event_pb2_grpc

# ────────────────────────── construction ────────────────────────


class TestClientConstruction:
    def test_socket_path_required(self):
        with pytest.raises(ValueError):
            Client("")

    def test_records_socket_path(self):
        c = Client("/tmp/nerv-test.sock")
        try:
            assert c.socket_path == "/tmp/nerv-test.sock"
        finally:
            c.close()

    def test_context_manager_closes(self):
        with Client("/tmp/nerv-test.sock") as c:
            assert c.socket_path == "/tmp/nerv-test.sock"
        # No assertion on channel state — close() is idempotent in grpc.

    def test_from_channel_skips_socket_dial(self):
        # An invalid `unix:` path would fail on first RPC; from_channel
        # bypasses construction entirely, so this must not raise.
        channel = grpc.insecure_channel("localhost:1")
        c = Client.from_channel(channel)
        try:
            assert c.socket_path == ""
        finally:
            c.close()


# ─────────────────── in-process fake daemon ─────────────────────


class _FakeService(event_pb2_grpc.EventServiceServicer):
    """In-process EventService. Configurable per-test via instance flags."""

    def __init__(self) -> None:
        self.dispatches = 0
        self.lists = 0
        self.raise_dispatch: grpc.StatusCode | None = None

    def Dispatch(self, request, context):
        self.dispatches += 1
        if self.raise_dispatch is not None:
            context.abort(self.raise_dispatch, "denied")
        msg = "ok"
        env = request.envelope
        if env is not None and env.event:
            msg = f"received:{env.event}"
        return event_pb2.DispatchResponse(
            decision=event_pb2.Decision(
                action=event_pb2.ACTION_ALLOW,
                message=msg,
            ),
        )

    def List(self, request, context):
        self.lists += 1
        return event_pb2.ListResponse(
            events=[
                event_pb2.EventDescriptor(
                    name="PreToolUse",
                    category="tool",
                    origin="native",
                    blocking=True,
                ),
                event_pb2.EventDescriptor(
                    name="SessionStart",
                    category="session",
                    origin="native",
                ),
            ],
        )


@pytest.fixture
def fake_daemon() -> Iterator[tuple[str, _FakeService]]:
    """Yield (socket_path, service) bound to an in-process gRPC server."""
    tmpdir = tempfile.mkdtemp(prefix="nerv-py-")
    socket_path = os.path.join(tmpdir, "n.sock")
    service = _FakeService()
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=4))
    event_pb2_grpc.add_EventServiceServicer_to_server(service, server)
    server.add_insecure_port(f"unix:{socket_path}")
    server.start()
    try:
        yield socket_path, service
    finally:
        server.stop(grace=None).wait()
        try:
            os.unlink(socket_path)
        except FileNotFoundError:
            pass
        os.rmdir(tmpdir)


# ───────────────────── dispatch + list ──────────────────────────


class TestClientDispatch:
    def test_round_trip_over_uds(self, fake_daemon):
        socket_path, service = fake_daemon
        with Client(socket_path) as c:
            envelope = event_pb2.Envelope(
                nerv_version="1.0.0-draft",
                event="PreToolUse",
                event_id="00000000-0000-0000-0000-000000000000",
            )
            decision = c.dispatch(envelope)
            assert decision.action == event_pb2.ACTION_ALLOW
            assert decision.message == "received:PreToolUse"
            assert service.dispatches == 1

    def test_rpc_error_propagates(self, fake_daemon):
        socket_path, service = fake_daemon
        service.raise_dispatch = grpc.StatusCode.PERMISSION_DENIED
        with Client(socket_path) as c:
            envelope = event_pb2.Envelope(
                nerv_version="1.0.0-draft",
                event="PreToolUse",
                event_id="11111111-1111-1111-1111-111111111111",
            )
            with pytest.raises(grpc.RpcError) as excinfo:
                c.dispatch(envelope)
            assert excinfo.value.code() == grpc.StatusCode.PERMISSION_DENIED

    def test_dispatch_wraps_envelope_with_mock_stub(self):
        # Unit-test seam: confirms the request shaping without a server.
        c = Client("/tmp/nerv-test.sock")
        try:
            stub = MagicMock()
            decision = event_pb2.Decision(action=event_pb2.ACTION_ALLOW)
            stub.Dispatch.return_value = event_pb2.DispatchResponse(
                decision=decision,
            )
            c._stub = stub

            envelope = event_pb2.Envelope(event="PreToolUse")
            result = c.dispatch(envelope)

            assert result.action == event_pb2.ACTION_ALLOW
            stub.Dispatch.assert_called_once()
            sent = stub.Dispatch.call_args.args[0]
            assert isinstance(sent, event_pb2.DispatchRequest)
            assert sent.envelope.event == "PreToolUse"
        finally:
            c.close()


class TestClientList:
    def test_round_trip_over_uds(self, fake_daemon):
        socket_path, service = fake_daemon
        with Client(socket_path) as c:
            out = c.list()
            assert [e.name for e in out] == ["PreToolUse", "SessionStart"]
            assert service.lists == 1

    def test_list_returns_list_of_descriptors_mock(self):
        c = Client("/tmp/nerv-test.sock")
        try:
            stub = MagicMock()
            descriptors = [
                event_pb2.EventDescriptor(name="PreToolUse", category="tool"),
                event_pb2.EventDescriptor(name="SessionStart", category="session"),
            ]
            stub.List.return_value = event_pb2.ListResponse(events=descriptors)
            c._stub = stub

            out = c.list()
            assert isinstance(out, list)
            assert [e.name for e in out] == ["PreToolUse", "SessionStart"]
        finally:
            c.close()


# ───────────────────── close idempotency ────────────────────────


class TestClientClose:
    def test_close_idempotent(self):
        c = Client("/tmp/nerv-test.sock")
        c.close()
        c.close()  # must not raise

    def test_close_from_channel_client(self):
        channel = grpc.insecure_channel("localhost:1")
        c = Client.from_channel(channel)
        c.close()
        c.close()


# ──────────────────── default_socket_path ───────────────────────


class TestDefaultSocketPath:
    def test_respects_env_override(self, monkeypatch):
        monkeypatch.setenv(ENV_SOCKET, "/custom/path.sock")
        assert default_socket_path() == "/custom/path.sock"

    def test_xdg_runtime_dir_fallback(self, monkeypatch):
        monkeypatch.delenv(ENV_SOCKET, raising=False)
        monkeypatch.setenv("XDG_RUNTIME_DIR", "/run/user/1000")
        assert default_socket_path() == "/run/user/1000/nerv.sock"

    def test_tmpdir_when_xdg_unset(self, monkeypatch):
        monkeypatch.delenv(ENV_SOCKET, raising=False)
        monkeypatch.delenv("XDG_RUNTIME_DIR", raising=False)
        expected = os.path.join(tempfile.gettempdir(), "nerv.sock")
        assert default_socket_path() == expected

    def test_empty_env_treated_as_unset(self, monkeypatch):
        monkeypatch.setenv(ENV_SOCKET, "")
        monkeypatch.setenv("XDG_RUNTIME_DIR", "/run/user/1000")
        assert default_socket_path() == "/run/user/1000/nerv.sock"


# ──────────────────────── gen package ───────────────────────────


class TestGenPackage:
    def test_proto_types_importable(self):
        # Re-imported here to confirm the gen sys.path shim still works
        # after Client construction.
        from nerv.v1 import event_pb2 as ev

        env = ev.Envelope(event="Stop")
        assert env.event == "Stop"
