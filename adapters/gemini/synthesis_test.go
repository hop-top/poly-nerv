package gemini

import (
	"testing"

	"hop.top/nerv/internal/events"
)

func TestSynthesizeFromBeforeTool_TaskCreated(t *testing.T) {
	payload := map[string]any{
		"tool_name": "TaskCreate",
		"tool_input": map[string]any{
			"title": "implement feature X",
		},
	}
	synth := SynthesizeFromBeforeTool(payload)
	if !hasSynthEvent(synth, events.EventTaskCreated) {
		t.Error("expected TaskCreated synthesis")
	}
}

func TestSynthesizeFromBeforeTool_Elicitation(t *testing.T) {
	payload := map[string]any{
		"tool_name": "mcp__server__tool",
		"tool_input": map[string]any{
			"prompt": "enter value",
		},
	}
	synth := SynthesizeFromBeforeTool(payload)
	if !hasSynthEvent(synth, events.EventElicitation) {
		t.Error("expected Elicitation synthesis")
	}
}

func TestSynthesizeFromBeforeTool_NoMatch(t *testing.T) {
	payload := map[string]any{
		"tool_name":  "Shell",
		"tool_input": map[string]any{"command": "ls"},
	}
	synth := SynthesizeFromBeforeTool(payload)
	if len(synth) != 0 {
		t.Errorf("expected no synthesis, got %d", len(synth))
	}
}

func TestSynthesizeFromAfterTool_CwdChanged(t *testing.T) {
	payload := map[string]any{
		"tool_name": "Bash",
		"tool_input": map[string]any{
			"command": "cd /tmp/project",
		},
	}
	synth := SynthesizeFromAfterTool(payload)
	if !hasSynthEvent(synth, events.EventCwdChanged) {
		t.Error("expected CwdChanged synthesis")
	}
}

func TestSynthesizeFromAfterTool_WorktreeCreate(t *testing.T) {
	tests := []string{
		"git worktree add feature-branch",
		"git hop add feature-branch",
	}
	for _, cmd := range tests {
		t.Run(cmd, func(t *testing.T) {
			payload := map[string]any{
				"tool_name": "Shell",
				"tool_input": map[string]any{
					"command": cmd,
				},
			}
			synth := SynthesizeFromAfterTool(payload)
			if !hasSynthEvent(synth, events.EventWorktreeCreate) {
				t.Error("expected WorktreeCreate synthesis")
			}
		})
	}
}

func TestSynthesizeFromAfterTool_WorktreeRemove(t *testing.T) {
	tests := []string{
		"git worktree remove feature-branch",
		"git hop remove feature-branch",
	}
	for _, cmd := range tests {
		t.Run(cmd, func(t *testing.T) {
			payload := map[string]any{
				"tool_name": "Shell",
				"tool_input": map[string]any{
					"command": cmd,
				},
			}
			synth := SynthesizeFromAfterTool(payload)
			if !hasSynthEvent(synth, events.EventWorktreeRemove) {
				t.Error("expected WorktreeRemove synthesis")
			}
		})
	}
}

func TestSynthesizeFromAfterTool_InstructionsLoaded(t *testing.T) {
	payload := map[string]any{
		"tool_name": "Read",
		"tool_input": map[string]any{
			"path": "/home/user/project/CLAUDE.md",
		},
	}
	synth := SynthesizeFromAfterTool(payload)
	if !hasSynthEvent(synth, events.EventInstructionsLoaded) {
		t.Error("expected InstructionsLoaded synthesis")
	}
}

func TestSynthesizeFromAfterTool_ConfigChange(t *testing.T) {
	payload := map[string]any{
		"tool_name": "Write",
		"tool_input": map[string]any{
			"path": "/home/user/.gemini/settings.json",
		},
	}
	synth := SynthesizeFromAfterTool(payload)
	if !hasSynthEvent(synth, events.EventConfigChange) {
		t.Error("expected ConfigChange synthesis")
	}
	if !hasSynthEvent(synth, events.EventFileChanged) {
		t.Error("expected FileChanged synthesis from Write")
	}
}

func TestSynthesizeFromAfterTool_PostToolUseFailure(t *testing.T) {
	payload := map[string]any{
		"tool_name": "Shell",
		"tool_input": map[string]any{
			"command": "false",
		},
		"tool_response": map[string]any{
			"error": "exit status 1",
		},
	}
	synth := SynthesizeFromAfterTool(payload)
	if !hasSynthEvent(synth, events.EventPostToolUseFailure) {
		t.Error("expected PostToolUseFailure synthesis")
	}
}

func TestSynthesizeFromAfterTool_TaskCompleted(t *testing.T) {
	payload := map[string]any{
		"tool_name": "TaskUpdate",
		"tool_input": map[string]any{
			"id":     "42",
			"status": "completed",
		},
	}
	synth := SynthesizeFromAfterTool(payload)
	if !hasSynthEvent(synth, events.EventTaskCompleted) {
		t.Error("expected TaskCompleted synthesis")
	}
}

func TestSynthesizeFromAfterTool_ElicitationResult(t *testing.T) {
	payload := map[string]any{
		"tool_name": "mcp__server__tool",
		"tool_response": map[string]any{
			"result": "user input",
		},
	}
	synth := SynthesizeFromAfterTool(payload)
	if !hasSynthEvent(synth, events.EventElicitationResult) {
		t.Error("expected ElicitationResult synthesis")
	}
}

func TestSynthesizeFromAfterTool_EmptyToolName(t *testing.T) {
	synth := SynthesizeFromAfterTool(map[string]any{})
	if len(synth) != 0 {
		t.Errorf("expected no synthesis for empty payload, got %d", len(synth))
	}
}

// hasSynthEvent checks if a slice contains a synthetic event of the
// given type.
func hasSynthEvent(ss []SyntheticEvent, e events.Event) bool {
	for _, s := range ss {
		if s.Event == e {
			return true
		}
	}
	return false
}
