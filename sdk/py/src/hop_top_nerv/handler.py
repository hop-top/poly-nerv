"""nerv handler — read envelope from stdin, call handler, write decision."""

from __future__ import annotations

import json
import sys
from collections.abc import Callable
from dataclasses import dataclass
from typing import Any


@dataclass
class CliInfo:
    """Metadata about the CLI that produced this event."""

    name: str
    version: str
    binary_path: str | None = None


@dataclass
class SessionInfo:
    """Session context for this event."""

    id: str
    project_key: str | None = None
    cwd: str | None = None


@dataclass
class Envelope:
    """nerv envelope — wire format delivered on stdin."""

    nerv_version: str
    event: str
    event_id: str
    timestamp: str
    cli: CliInfo
    session: SessionInfo
    payload: dict[str, Any]
    synthesized: bool = False
    synthesis_source: str | None = None
    capabilities: dict[str, Any] | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Envelope:
        """Parse a raw dict (from JSON) into an Envelope."""
        return cls(
            nerv_version=data["nerv_version"],
            event=data["event"],
            event_id=data["event_id"],
            timestamp=data["timestamp"],
            cli=CliInfo(**data["cli"]),
            session=SessionInfo(**data["session"]),
            payload=data.get("payload", {}),
            synthesized=data.get("synthesized", False),
            synthesis_source=data.get("synthesis_source"),
            capabilities=data.get("capabilities"),
        )


@dataclass
class Decision:
    """nerv decision — written to stdout."""

    action: str  # allow | warn | block | rewrite
    message: str | None = None
    rewrite: dict[str, Any] | None = None
    metadata: dict[str, Any] | None = None

    def to_dict(self) -> dict[str, Any]:
        """Serialize to dict, omitting None fields."""
        d: dict[str, Any] = {"action": self.action}
        if self.message is not None:
            d["message"] = self.message
        if self.rewrite is not None:
            d["rewrite"] = self.rewrite
        if self.metadata is not None:
            d["metadata"] = self.metadata
        return d


_EXIT_CODES = {
    "allow": 0,
    "warn": 1,
    "block": 2,
    "rewrite": 3,
}


def create_handler(
    fn: Callable[[Envelope], Decision],
) -> None:
    """Read envelope from stdin, call fn, write decision to stdout.

    Exits with the code matching the decision action:
    allow=0, warn=1, block=2, rewrite=3, error>=4.
    """
    try:
        raw = sys.stdin.read()
        envelope = Envelope.from_dict(json.loads(raw))
        decision = fn(envelope)
        sys.stdout.write(json.dumps(decision.to_dict()) + "\n")
        sys.stdout.flush()
        exit_code = _EXIT_CODES.get(decision.action)
        if exit_code is None:
            sys.stderr.write(
                f"nerv-sdk: invalid action: {decision.action!r}\n"
            )
            sys.exit(4)
        sys.exit(exit_code)
    except Exception as exc:  # noqa: BLE001 -- any handler failure maps to exit 4
        sys.stderr.write(f"nerv-sdk: handler error: {exc}\n")
        sys.exit(4)
