"""Tests for hop_top_nerv.events."""

from __future__ import annotations

from hop_top_nerv import events
from hop_top_nerv.events import (
    EVENTS,
    INBOX_MESSAGE_ADDED,
    PRE_TOOL_USE,
    SESSION_START,
)


def test_events_returns_frozenset():
    assert isinstance(events(), frozenset)
    assert events() is EVENTS


def test_well_known_events_present():
    assert PRE_TOOL_USE in EVENTS
    assert SESSION_START in EVENTS
    assert INBOX_MESSAGE_ADDED in EVENTS


def test_inbox_events_are_snake_case():
    assert INBOX_MESSAGE_ADDED == "inbox_message_added"


def test_event_count_matches_spec():
    # 34 events per events.py header.
    assert len(EVENTS) == 34
