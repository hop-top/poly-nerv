package goose

import (
	"strings"
	"testing"
)

// --- Issue 3: synthesis_source always "extension" ---

func TestRegression_SynthesisSource_TasksUpdate(t *testing.T) {
	env, err := ToEnvelope(map[string]any{
		"type":   "tasks_update",
		"status": "Running",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.SynthesisSource == nil {
		t.Fatal("synthesis_source should not be nil")
	}
	if *env.SynthesisSource != "notification" {
		t.Errorf("synthesis_source = %q, want %q",
			*env.SynthesisSource, "notification")
	}
}

func TestRegression_SynthesisSource_CwdChanged(t *testing.T) {
	env, err := ToEnvelope(map[string]any{
		"type": "cwd.changed",
		"cwd":  "/new/dir",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.SynthesisSource == nil {
		t.Fatal("synthesis_source should not be nil")
	}
	if *env.SynthesisSource != "shell-match" {
		t.Errorf("synthesis_source = %q, want %q",
			*env.SynthesisSource, "shell-match")
	}
}

func TestRegression_SynthesisSource_Error(t *testing.T) {
	// "error" is a native event, so it should NOT be synthesized.
	// But if it were synthesized, it should be "sse", not "extension".
	// Actually, "error" is in nativeEvents, so Synthesized=false.
	// This test verifies it stays native (no synthesis_source).
	env, err := ToEnvelope(map[string]any{
		"type":  "error",
		"error": "API timeout",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Synthesized {
		t.Error("error event should be native (not synthesized)")
	}
	if env.SynthesisSource != nil {
		t.Error("native event should have nil synthesis_source")
	}
}

func TestRegression_SynthesisSource_RequestStart(t *testing.T) {
	env, err := ToEnvelope(map[string]any{
		"type": "request.start",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.SynthesisSource == nil {
		t.Fatal("synthesis_source should not be nil")
	}
	if *env.SynthesisSource != "sse" {
		t.Errorf("synthesis_source = %q, want %q",
			*env.SynthesisSource, "sse")
	}
}

func TestRegression_SynthesisSource_WorktreeCreated(t *testing.T) {
	env, err := ToEnvelope(map[string]any{
		"type": "worktree.created",
		"path": "/tmp/wt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.SynthesisSource == nil {
		t.Fatal("synthesis_source should not be nil")
	}
	if *env.SynthesisSource != "shell-match" {
		t.Errorf("synthesis_source = %q, want %q",
			*env.SynthesisSource, "shell-match")
	}
}

// --- Issue 7: duplicate extensions: YAML key ---

func TestRegression_MergeConfig_DuplicateExtensions(t *testing.T) {
	existing := []byte(`extensions:
  other-ext:
    enabled: true
`)
	a := &Adapter{}
	merged, err := a.MergeConfig(existing)
	if err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}
	content := string(merged)

	// Must NOT have two "extensions:" lines.
	count := strings.Count(content, "extensions:")
	if count > 1 {
		t.Errorf("found %d 'extensions:' lines, want 1:\n%s",
			count, content)
	}

	// nerv-bridge must be present.
	if !strings.Contains(content, "nerv-bridge") {
		t.Error("merged config missing nerv-bridge")
	}

	// other-ext must still be present.
	if !strings.Contains(content, "other-ext") {
		t.Error("merged config lost other-ext")
	}
}

// --- Issue 9: tasks_update mapped too broadly to TaskCreated ---

func TestRegression_TasksUpdate_CompletedNotTaskCreated(t *testing.T) {
	env, err := ToEnvelope(map[string]any{
		"type":   "tasks_update",
		"status": "Completed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Event == "TaskCreated" {
		t.Error("tasks_update with status=Completed should NOT " +
			"produce TaskCreated")
	}
}

func TestRegression_TasksUpdate_RunningIsTaskCreated(t *testing.T) {
	env, err := ToEnvelope(map[string]any{
		"type":   "tasks_update",
		"status": "Running",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Event != "TaskCreated" {
		t.Errorf("event = %q, want TaskCreated for Running status",
			env.Event)
	}
}

func TestRegression_TasksUpdate_PendingNotTaskCreated(t *testing.T) {
	env, err := ToEnvelope(map[string]any{
		"type":   "tasks_update",
		"status": "Pending",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Event == "TaskCreated" {
		t.Error("tasks_update with status=Pending should NOT " +
			"produce TaskCreated")
	}
}
