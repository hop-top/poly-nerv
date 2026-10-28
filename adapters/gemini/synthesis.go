package gemini

import (
	"encoding/json"
	"regexp"
	"strings"

	"hop.top/nerv/internal/events"
)

// SyntheticEvent represents a nerv event synthesized from a Gemini
// tool event by inspecting tool name and arguments.
type SyntheticEvent struct {
	Event   events.Event
	Payload map[string]any
}

// Regex patterns for synthesis matchers.
var (
	cdPattern      = regexp.MustCompile(`(?m)^\s*cd\s+`)
	worktreeAddPat = regexp.MustCompile(
		`(?:git\s+worktree\s+add|git\s+hop\s+add)`,
	)
	worktreeRmPat = regexp.MustCompile(
		`(?:git\s+worktree\s+remove|git\s+hop\s+remove)`,
	)
	instructionsPat = regexp.MustCompile(
		`(?i)(?:CLAUDE|AGENTS)\.md|rules/.*\.md`,
	)
	settingsPathPat = regexp.MustCompile(
		`(?i)settings\.json|config\.ya?ml`,
	)
	mcpToolPat = regexp.MustCompile(`^mcp__`)
)

// SynthesizeFromBeforeTool inspects a BeforeTool payload and returns
// zero or more synthetic nerv events. Checks for:
//   - PermissionRequest (tool call needing approval)
//   - TaskCreated (TaskCreate/todo_write tool)
//   - Elicitation (mcp__* tool)
func SynthesizeFromBeforeTool(payload map[string]any) []SyntheticEvent {
	toolName := strVal(payload, "tool_name")
	if toolName == "" {
		return nil
	}

	var out []SyntheticEvent

	// TaskCreated: tool name matches task creation tools.
	if isTaskCreateTool(toolName) {
		out = append(out, SyntheticEvent{
			Event: events.EventTaskCreated,
			Payload: map[string]any{
				"tool_name":  toolName,
				"tool_input": payload["tool_input"],
			},
		})
	}

	// Elicitation: MCP tool prefix.
	if mcpToolPat.MatchString(toolName) {
		out = append(out, SyntheticEvent{
			Event: events.EventElicitation,
			Payload: map[string]any{
				"tool_name":  toolName,
				"tool_input": payload["tool_input"],
			},
		})
	}

	return out
}

// SynthesizeFromAfterTool inspects an AfterTool payload and returns
// zero or more synthetic nerv events. Checks for:
//   - PostToolUseFailure (error in response)
//   - CwdChanged (cd command in Bash/Shell)
//   - WorktreeCreate/Remove (git worktree commands)
//   - InstructionsLoaded (Read of CLAUDE.md/AGENTS.md)
//   - ConfigChange (Write to settings files)
//   - FileChanged (Write/Edit tool)
//   - TaskCompleted (TaskUpdate with completed status)
//   - ElicitationResult (mcp__* tool)
func SynthesizeFromAfterTool(payload map[string]any) []SyntheticEvent {
	toolName := strVal(payload, "tool_name")
	if toolName == "" {
		return nil
	}

	var out []SyntheticEvent

	// PostToolUseFailure: error field present.
	if hasToolError(payload) {
		out = append(out, SyntheticEvent{
			Event: events.EventPostToolUseFailure,
			Payload: map[string]any{
				"tool_name": toolName,
				"error":     extractError(payload),
			},
		})
	}

	// Shell/Bash command analysis.
	if isShellTool(toolName) {
		cmd := extractCommand(payload)
		if cmd != "" {
			out = append(out, synthFromCommand(cmd)...)
		}
	}

	// Read tool: check for instructions files.
	if isReadTool(toolName) {
		path := extractPath(payload)
		if instructionsPat.MatchString(path) {
			out = append(out, SyntheticEvent{
				Event: events.EventInstructionsLoaded,
				Payload: map[string]any{
					"path": path,
				},
			})
		}
	}

	// Write/Edit tool: file change + config change detection.
	if isWriteTool(toolName) {
		path := extractPath(payload)
		if path != "" {
			out = append(out, SyntheticEvent{
				Event: events.EventFileChanged,
				Payload: map[string]any{
					"path":      path,
					"tool_name": toolName,
				},
			})
		}
		if path != "" && settingsPathPat.MatchString(path) {
			out = append(out, SyntheticEvent{
				Event: events.EventConfigChange,
				Payload: map[string]any{
					"path": path,
				},
			})
		}
	}

	// TaskCompleted: TaskUpdate tool with status change.
	if isTaskCompleteTool(toolName) {
		if hasCompletedStatus(payload) {
			out = append(out, SyntheticEvent{
				Event: events.EventTaskCompleted,
				Payload: map[string]any{
					"tool_name":  toolName,
					"tool_input": payload["tool_input"],
				},
			})
		}
	}

	// ElicitationResult: MCP tool response.
	if mcpToolPat.MatchString(toolName) {
		out = append(out, SyntheticEvent{
			Event: events.EventElicitationResult,
			Payload: map[string]any{
				"tool_name":     toolName,
				"tool_response": payload["tool_response"],
			},
		})
	}

	return out
}

// synthFromCommand checks a shell command string for CwdChanged,
// WorktreeCreate, and WorktreeRemove patterns.
func synthFromCommand(cmd string) []SyntheticEvent {
	var out []SyntheticEvent

	if cdPattern.MatchString(cmd) {
		out = append(out, SyntheticEvent{
			Event:   events.EventCwdChanged,
			Payload: map[string]any{"command": cmd},
		})
	}
	if worktreeAddPat.MatchString(cmd) {
		out = append(out, SyntheticEvent{
			Event:   events.EventWorktreeCreate,
			Payload: map[string]any{"command": cmd},
		})
	}
	if worktreeRmPat.MatchString(cmd) {
		out = append(out, SyntheticEvent{
			Event:   events.EventWorktreeRemove,
			Payload: map[string]any{"command": cmd},
		})
	}

	return out
}

// --- helpers ---

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func isShellTool(name string) bool {
	n := strings.ToLower(name)
	return n == "bash" || n == "shell" || n == "local_shell"
}

func isReadTool(name string) bool {
	n := strings.ToLower(name)
	return n == "read" || n == "readfile" || n == "read_file"
}

func isWriteTool(name string) bool {
	n := strings.ToLower(name)
	return n == "write" || n == "edit" || n == "writefile" ||
		n == "write_file" || n == "apply_patch"
}

func isTaskCreateTool(name string) bool {
	n := strings.ToLower(name)
	return n == "taskcreate" || n == "task_create" || n == "todo_write"
}

func isTaskCompleteTool(name string) bool {
	n := strings.ToLower(name)
	return n == "taskupdate" || n == "task_update" || n == "todo_update"
}

func hasToolError(m map[string]any) bool {
	if resp, ok := m["tool_response"].(map[string]any); ok {
		if _, hasErr := resp["error"]; hasErr {
			return true
		}
	}
	return false
}

func extractError(m map[string]any) any {
	if resp, ok := m["tool_response"].(map[string]any); ok {
		return resp["error"]
	}
	return nil
}

func extractCommand(m map[string]any) string {
	// Try tool_input.command first.
	if ti, ok := m["tool_input"].(map[string]any); ok {
		if cmd, ok := ti["command"].(string); ok {
			return cmd
		}
	}
	// Try raw JSON tool_input.
	if raw, ok := m["tool_input"].(json.RawMessage); ok {
		var ti map[string]any
		if json.Unmarshal(raw, &ti) == nil {
			if cmd, ok := ti["command"].(string); ok {
				return cmd
			}
		}
	}
	return ""
}

func extractPath(m map[string]any) string {
	if ti, ok := m["tool_input"].(map[string]any); ok {
		if p, ok := ti["path"].(string); ok {
			return p
		}
		if p, ok := ti["file_path"].(string); ok {
			return p
		}
	}
	return ""
}

// TODO: implement PermissionRequest synthesis from BeforeTool exit=2.
// BeforeTool returning exit 2 (block) is a permission denial; the adapter
// should emit PermissionRequest before dispatching and PermissionDenied
// after exit=2 is observed.

// TODO: implement PostCompact synthesis from AfterModel with compaction
// marker. AfterModel fires for every model call; only emit PostCompact
// when the response contains a compaction indicator.

func hasCompletedStatus(m map[string]any) bool {
	if ti, ok := m["tool_input"].(map[string]any); ok {
		if s, ok := ti["status"].(string); ok {
			return strings.EqualFold(s, "completed") ||
				strings.EqualFold(s, "done")
		}
	}
	return false
}
