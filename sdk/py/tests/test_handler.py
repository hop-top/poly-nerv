"""Tests for hop_top_nerv.handler."""

from __future__ import annotations

import json

from hop_top_nerv.handler import Decision, Envelope

SAMPLE_ENVELOPE_DICT = {
    "nerv_version": "1.0.0-draft",
    "event": "PreToolUse",
    "event_id": "550e8400-e29b-41d4-a716-446655440000",
    "timestamp": "2026-04-11T12:00:00Z",
    "cli": {"name": "claude-code", "version": "1.0.0"},
    "session": {"id": "abc-123", "cwd": "/tmp"},
    "payload": {"tool_name": "Bash", "tool_input": {"command": "ls"}},
    "synthesized": False,
}


class TestEnvelope:
    def test_from_dict_required_fields(self):
        env = Envelope.from_dict(SAMPLE_ENVELOPE_DICT)
        assert env.event == "PreToolUse"
        assert env.cli.name == "claude-code"
        assert env.session.id == "abc-123"
        assert env.payload["tool_name"] == "Bash"
        assert env.synthesized is False
        assert env.synthesis_source is None

    def test_from_dict_with_optional_fields(self):
        data = {**SAMPLE_ENVELOPE_DICT, "synthesis_source": "matcher"}
        env = Envelope.from_dict(data)
        assert env.synthesis_source == "matcher"

    def test_from_dict_with_binary_path(self):
        data = dict(SAMPLE_ENVELOPE_DICT)
        data["cli"] = {
            "name": "claude-code",
            "version": "1.0.0",
            "binary_path": "/usr/local/bin/claude",
        }
        env = Envelope.from_dict(data)
        assert env.cli.binary_path == "/usr/local/bin/claude"


class TestDecision:
    def test_allow(self):
        d = Decision(action="allow")
        assert d.to_dict() == {"action": "allow"}

    def test_warn_with_message(self):
        d = Decision(action="warn", message="Careful!")
        assert d.to_dict() == {"action": "warn", "message": "Careful!"}

    def test_block_with_message(self):
        d = Decision(action="block", message="Not allowed")
        out = d.to_dict()
        assert out["action"] == "block"
        assert out["message"] == "Not allowed"

    def test_rewrite_with_payload(self):
        rewrite = {"tool_input": {"command": "ls -la"}}
        d = Decision(action="rewrite", rewrite=rewrite)
        out = d.to_dict()
        assert out["action"] == "rewrite"
        assert out["rewrite"] == rewrite

    def test_metadata_included(self):
        d = Decision(action="allow", metadata={"reason": "safe"})
        out = d.to_dict()
        assert out["metadata"] == {"reason": "safe"}

    def test_none_fields_omitted(self):
        d = Decision(action="allow")
        out = d.to_dict()
        assert "message" not in out
        assert "rewrite" not in out
        assert "metadata" not in out

    def test_roundtrip_json(self):
        d = Decision(action="warn", message="heads up")
        raw = json.dumps(d.to_dict())
        parsed = json.loads(raw)
        assert parsed == {"action": "warn", "message": "heads up"}
