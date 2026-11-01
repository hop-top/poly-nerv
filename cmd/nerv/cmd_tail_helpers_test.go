package main

import (
	"strings"
	"testing"
	"time"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/eventlog"
)

func tailEntry(event, cli string, totalMS int64, action string) eventlog.Entry {
	e := eventlog.Entry{
		Envelope: &envelope.Envelope{
			Event:     event,
			Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.UTC),
			CLI:       envelope.CLIInfo{Name: cli},
		},
	}
	if action != "" {
		e.Decision = map[string]any{"action": action}
	}
	if totalMS >= 0 {
		e.Timings = map[string]int64{"total_ms": totalMS}
	}
	return e
}

func TestEntryToRow(t *testing.T) {
	row := entryToRow(tailEntry("PreToolUse", "claude", 42, "block"))
	want := tailRow{
		Time: "03:04:05.006", Event: "PreToolUse", CLI: "claude",
		Decision: "block", Duration: "42ms",
	}
	if row != want {
		t.Errorf("row = %+v, want %+v", row, want)
	}

	// No envelope/decision/timings: placeholders, no decision/duration.
	empty := entryToRow(eventlog.Entry{})
	if empty.Time != "—" || empty.Event != "—" || empty.CLI != "—" {
		t.Errorf("placeholders missing: %+v", empty)
	}
	if empty.Decision != "" || empty.Duration != "" {
		t.Errorf("unexpected decision/duration: %+v", empty)
	}

	// Decision that is not a map is ignored rather than panicking.
	e := tailEntry("Stop", "codex", -1, "")
	e.Decision = "allow"
	if got := entryToRow(e); got.Decision != "" || got.Duration != "" {
		t.Errorf("non-map decision leaked: %+v", got)
	}
}

func TestMatchesFilter(t *testing.T) {
	e := tailEntry("PreToolUse", "claude", -1, "")
	cases := []struct {
		name       string
		e          eventlog.Entry
		event, cli string
		want       bool
	}{
		{"no filters", e, "", "", true},
		{"event match", e, "PreToolUse", "", true},
		{"event mismatch", e, "Stop", "", false},
		{"cli match", e, "", "claude", true},
		{"cli mismatch", e, "", "codex", false},
		{"both match", e, "PreToolUse", "claude", true},
		{"event ok cli bad", e, "PreToolUse", "codex", false},
		{"nil envelope never matches", eventlog.Entry{}, "", "", false},
	}
	for _, tc := range cases {
		if got := matchesFilter(tc.e, tc.event, tc.cli); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestIsSlow(t *testing.T) {
	cases := []struct {
		name string
		e    eventlog.Entry
		want bool
	}{
		{"no timings", eventlog.Entry{}, false},
		{"missing total", eventlog.Entry{Timings: map[string]int64{"x": 999}}, false},
		{"exactly 100 is not slow", tailEntry("a", "b", 100, ""), false},
		{"101 is slow", tailEntry("a", "b", 101, ""), true},
	}
	for _, tc := range cases {
		if got := isSlow(tc.e); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestPrintTailLineAndRenderRow(t *testing.T) {
	out := captureStdout(t, func() {
		printTailLine(tailRow{Time: "t", Event: "E", CLI: "c"})
		printTailLine(tailRow{Time: "t", Event: "E", CLI: "c", Decision: "block", Duration: "5ms"})
	})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || lines[0] != "t  E  c" || lines[1] != "t  E  c  block  5ms" {
		t.Errorf("unexpected tail lines: %q", lines)
	}

	out = captureStdout(t, func() {
		renderRow("json", tailRow{Time: "t", Event: "E", CLI: "c", Decision: "allow", Duration: "1ms"})
	})
	for _, want := range []string{`"event"`, `"E"`, `"decision"`, `"allow"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json row missing %s: %s", want, out)
		}
	}
}
