//go:build !windows

package main

import (
	"strings"
	"testing"
)

// TestParseBindings_RejectsUnknownEvent locks in event-name
// validation against the canonical catalog. A binding like
// "foo:NotARealEvent" parses syntactically (one colon, two non-empty
// tokens) but the event name is not in events.IsValid — without the
// validator the bad pair would land in .nerv/disabled.yaml and never
// match anything at dispatch time.
func TestParseBindings_RejectsUnknownEvent(t *testing.T) {
	_, err := parseBindings([]string{"foo:NotARealEvent"})
	if err == nil {
		t.Fatal("expected error for unknown event, got nil")
	}
	if !strings.Contains(err.Error(), "unknown event") {
		t.Fatalf("expected 'unknown event' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "NotARealEvent") {
		t.Fatalf("expected offending event name in error, got: %v", err)
	}
}

// TestParseBindings_RejectsSecondColon guards the previously-silent
// truncation: with a naive first-colon split, "foo:Bar:Baz" would
// parse as Hook=foo, Event="Bar:Baz" (and then fail event validation,
// but with a confusing message). Reject the second colon explicitly.
func TestParseBindings_RejectsSecondColon(t *testing.T) {
	_, err := parseBindings([]string{"foo:Bar:Baz"})
	if err == nil {
		t.Fatal("expected error for second colon, got nil")
	}
	if !strings.Contains(err.Error(), "':'") {
		t.Fatalf("expected colon-related error, got: %v", err)
	}
}

// TestParseBindings_AcceptsCanonicalEvent confirms the validator is
// not over-strict — a real catalog event passes.
func TestParseBindings_AcceptsCanonicalEvent(t *testing.T) {
	got, err := parseBindings([]string{"my-hook:PreToolUse"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Hook != "my-hook" || got[0].Event != "PreToolUse" {
		t.Fatalf("parse result: %+v", got)
	}
}

// TestRunDisableEvents_RejectsUnknown locks in the same validation on
// the positional-event surface (nerv disable events <name>...). The
// catalog miss must error before OpenProject so a fresh cwd without a
// .nerv/ directory still surfaces the typo cleanly.
func TestRunDisableEvents_RejectsUnknown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	chdirTemp(t)

	err := runDisableEvents(disableEventsCmd(), []string{"NotARealEvent"}, true, true)
	if err == nil {
		t.Fatal("expected error for unknown event, got nil")
	}
	if !strings.Contains(err.Error(), "unknown event") {
		t.Fatalf("expected 'unknown event' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "NotARealEvent") {
		t.Fatalf("expected offending event name in error, got: %v", err)
	}
}
