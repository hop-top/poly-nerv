package claude

import (
	"encoding/json"
	"fmt"
)

// nervMarker identifies nerv-managed hook entries for idempotent merges.
const nervMarker = "nervd"

// nativeEvents lists the 25 Claude Code native events that nerv
// manages. WorktreeCreate is deliberately absent: Claude treats a
// registered WorktreeCreate hook as a delegation point — the hook must
// create the worktree and echo its path, and both decision JSON and
// empty stdout fail the creation. nerv observes; it cannot fulfill
// that contract.
var nativeEvents = []string{
	"SessionStart",
	"SessionEnd",
	"UserPromptSubmit",
	"PreToolUse",
	"PostToolUse",
	"PostToolUseFailure",
	"PermissionRequest",
	"PermissionDenied",
	"Notification",
	"SubagentStart",
	"SubagentStop",
	"TaskCreated",
	"TaskCompleted",
	"Stop",
	"StopFailure",
	"TeammateIdle",
	"InstructionsLoaded",
	"ConfigChange",
	"CwdChanged",
	"FileChanged",
	"WorktreeRemove",
	"PreCompact",
	"PostCompact",
	"Elicitation",
	"ElicitationResult",
}

// mergeHooksIntoSettings merges nerv hook entries into Claude's
// settings.json, preserving all non-nerv settings and hooks.
func mergeHooksIntoSettings(existing []byte) ([]byte, error) {
	settings := make(map[string]any)
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &settings); err != nil {
			return nil, fmt.Errorf(
				"claude: unmarshal settings: %w", err,
			)
		}
	}

	hooks := extractHooksMap(settings)
	for _, event := range nativeEvents {
		nervEntry := buildEventEntry(event)
		if existing, ok := hooks[event]; ok {
			if existingSlice, ok := existing.([]any); ok {
				hooks[event] = append(existingSlice, nervEntry...)
				continue
			}
		}
		hooks[event] = nervEntry
	}
	settings["hooks"] = hooks

	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf(
			"claude: marshal settings: %w", err,
		)
	}
	return append(out, '\n'), nil
}

// extractHooksMap returns the hooks map from settings, stripping any
// existing nerv entries. Creates a new map if none exists.
func extractHooksMap(settings map[string]any) map[string]any {
	raw, ok := settings["hooks"]
	if !ok {
		return make(map[string]any)
	}
	hooks, ok := raw.(map[string]any)
	if !ok {
		return make(map[string]any)
	}

	// Strip existing nerv entries while preserving user hooks.
	cleaned := make(map[string]any)
	for event, v := range hooks {
		entries, ok := v.([]any)
		if !ok {
			cleaned[event] = v
			continue
		}
		var kept []any
		for _, entry := range entries {
			if !isNervEntry(entry) {
				kept = append(kept, entry)
			}
		}
		if len(kept) > 0 {
			cleaned[event] = kept
		}
	}
	return cleaned
}

// isNervEntry checks if a hook entry was created by nerv (contains nervd
// command).
func isNervEntry(entry any) bool {
	m, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	hooksList, ok := m["hooks"].([]any)
	if !ok {
		return false
	}
	for _, h := range hooksList {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := hm["type"].(string)
		cmd, _ := hm["command"].(string)
		if typ == "command" && cmd == nervMarker {
			return true
		}
	}
	return false
}

// buildEventEntry creates a Claude hook entry pointing to nervd.
//
// Format:
//
//	{
//	  "matcher": "",
//	  "hooks": [{"type": "command", "command": "nervd"}]
//	}
func buildEventEntry(event string) []any {
	_ = event // event name available for future per-event config
	return []any{
		map[string]any{
			"matcher": "",
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": nervMarker,
				},
			},
		},
	}
}
