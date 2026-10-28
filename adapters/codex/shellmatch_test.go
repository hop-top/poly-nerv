package codex

import (
	"testing"

	"hop.top/nerv/internal/events"
)

func TestShellMatch_CdCommand(t *testing.T) {
	m := NewShellMatcher()
	got := m.Match("cd /tmp/project")
	if len(got) != 1 {
		t.Fatalf("Match(cd) returned %d events, want 1", len(got))
	}
	if got[0].Event != events.EventCwdChanged {
		t.Errorf("event = %q, want CwdChanged", got[0].Event)
	}
}

func TestShellMatch_WorktreeAdd(t *testing.T) {
	m := NewShellMatcher()
	got := m.Match("git worktree add feature-branch")
	if len(got) != 1 {
		t.Fatalf("Match(worktree add) returned %d events, want 1", len(got))
	}
	if got[0].Event != events.EventWorktreeCreate {
		t.Errorf("event = %q, want WorktreeCreate", got[0].Event)
	}
}

func TestShellMatch_WorktreeRemove(t *testing.T) {
	m := NewShellMatcher()

	for _, cmd := range []string{
		"git worktree remove feature-branch",
		"git worktree rm feature-branch",
	} {
		got := m.Match(cmd)
		if len(got) != 1 {
			t.Fatalf("Match(%q) returned %d events, want 1", cmd, len(got))
		}
		if got[0].Event != events.EventWorktreeRemove {
			t.Errorf("event = %q, want WorktreeRemove", got[0].Event)
		}
	}
}

func TestShellMatch_InstructionsLoaded(t *testing.T) {
	m := NewShellMatcher()
	got := m.Match("cat /path/to/CLAUDE.md")
	if len(got) != 1 {
		t.Fatalf("Match(cat CLAUDE.md) returned %d events, want 1", len(got))
	}
	if got[0].Event != events.EventInstructionsLoaded {
		t.Errorf("event = %q, want InstructionsLoaded", got[0].Event)
	}
}

func TestShellMatch_NoMatch(t *testing.T) {
	m := NewShellMatcher()
	got := m.Match("ls -la /tmp")
	if len(got) != 0 {
		t.Errorf("Match(ls) returned %d events, want 0", len(got))
	}
}

func TestShellMatch_MultipleMatches(t *testing.T) {
	m := NewShellMatcher()
	got := m.Match("cd /tmp && git worktree add feat")
	if len(got) != 2 {
		t.Fatalf("Match(cd+worktree) returned %d events, want 2", len(got))
	}

	foundCwd := false
	foundWorktree := false
	for _, e := range got {
		switch e.Event {
		case events.EventCwdChanged:
			foundCwd = true
		case events.EventWorktreeCreate:
			foundWorktree = true
		}
	}
	if !foundCwd {
		t.Error("missing CwdChanged event")
	}
	if !foundWorktree {
		t.Error("missing WorktreeCreate event")
	}
}

func TestShellMatch_RmRf(t *testing.T) {
	m := NewShellMatcher()
	got := m.Match("rm -rf /important/dir")
	if len(got) != 1 {
		t.Fatalf("Match(rm -rf) returned %d events, want 1", len(got))
	}
	if got[0].Event != events.EventPermissionRequest {
		t.Errorf("event = %q, want PermissionRequest", got[0].Event)
	}
}
