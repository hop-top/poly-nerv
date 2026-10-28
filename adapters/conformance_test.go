//go:build conformance

// Package adapters_test runs conformance checks across all CLI adapters.
//
// Each adapter must satisfy the HookAdapter interface, return valid
// config paths, handle empty + existing config merges, and translate
// representative events correctly. Tests skip gracefully when the
// target CLI binary is not installed.
package adapters_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"hop.top/nerv/adapters/aider"
	"hop.top/nerv/adapters/claude"
	"hop.top/nerv/adapters/codex"
	"hop.top/nerv/adapters/gemini"
	"hop.top/nerv/adapters/goose"
	"hop.top/nerv/adapters/internal"
	"hop.top/nerv/adapters/opencode"
	"hop.top/nerv/adapters/openhands"
	"hop.top/nerv/adapters/vibe"
)

// adapterEntry bundles a HookAdapter with metadata for table-driven tests.
type adapterEntry struct {
	name             string
	adapter          internal.HookAdapter
	capabilitiesFile string
	// configFormat is "json" (default) or "toml".
	configFormat string
	// representative events the adapter should translate without error.
	translateInputs [][]byte
}

func allAdapters(t *testing.T) []adapterEntry {
	t.Helper()
	home := t.TempDir()

	return []adapterEntry{
		{
			name:             "aider",
			adapter:          &aider.Adapter{},
			capabilitiesFile: "aider/capabilities.yaml",
			configFormat:     "yaml",
			translateInputs: [][]byte{
				mustJSON(t, map[string]any{
					"hook_event_name": "PreToolUse",
					"session_id":      "conf-a1",
					"tool_name":       "edit",
				}),
				mustJSON(t, map[string]any{
					"hook_event_name": "SessionStart",
					"session_id":      "conf-a2",
				}),
				mustJSON(t, map[string]any{
					"hook_event_name": "PostToolUse",
					"session_id":      "conf-a3",
					"tool_name":       "test",
				}),
			},
		},
		{
			name:             "claude",
			adapter:          claude.NewAdapter(home),
			capabilitiesFile: "claude/capabilities.yaml",
			translateInputs: [][]byte{
				mustJSON(t, map[string]any{
					"hook_event_name": "PreToolUse",
					"session_id":      "conf-1",
					"tool_name":       "Bash",
					"tool_input":      map[string]any{"command": "ls"},
				}),
				mustJSON(t, map[string]any{
					"hook_event_name": "SessionStart",
					"session_id":      "conf-2",
				}),
				mustJSON(t, map[string]any{
					"hook_event_name": "PostToolUse",
					"session_id":      "conf-3",
					"tool_name":       "Read",
					"tool_input":      map[string]any{"file_path": "/tmp/x"},
				}),
			},
		},
		{
			name:             "gemini",
			adapter:          &gemini.Adapter{},
			capabilitiesFile: "gemini/capabilities.yaml",
			translateInputs: [][]byte{
				mustJSON(t, map[string]any{
					"hook_event_name": "BeforeTool",
					"session_id":      "conf-g1",
					"tool_name":       "shell",
				}),
				mustJSON(t, map[string]any{
					"hook_event_name": "SessionStart",
					"session_id":      "conf-g2",
				}),
				mustJSON(t, map[string]any{
					"hook_event_name": "AfterTool",
					"session_id":      "conf-g3",
					"tool_name":       "read_file",
				}),
			},
		},
		{
			name: "opencode",
			adapter: &opencode.Adapter{
				ProjectDir: home,
				NervdPath:  "/usr/local/bin/nervd",
			},
			capabilitiesFile: "opencode/capabilities.yaml",
			translateInputs: [][]byte{
				mustJSON(t, map[string]any{
					"type":       "tool.execute.before",
					"tool":       "bash",
					"session_id": "conf-o1",
					"input":      json.RawMessage(`{"command":"ls"}`),
				}),
				mustJSON(t, map[string]any{
					"type":       "session.created",
					"session_id": "conf-o2",
				}),
				mustJSON(t, map[string]any{
					"type":       "tool.execute.after",
					"tool":       "read",
					"session_id": "conf-o3",
					"output":     json.RawMessage(`"file contents"`),
				}),
			},
		},
		{
			name:             "goose",
			adapter:          &goose.Adapter{},
			capabilitiesFile: "goose/capabilities.yaml",
			configFormat:     "yaml",
			translateInputs: [][]byte{
				mustJSON(t, map[string]any{
					"type":       "tool.before",
					"session_id": "conf-gs1",
					"tool_name":  "shell",
				}),
				mustJSON(t, map[string]any{
					"type":       "session.start",
					"session_id": "conf-gs2",
				}),
				mustJSON(t, map[string]any{
					"type":       "tool.after",
					"session_id": "conf-gs3",
					"tool_name":  "read",
				}),
			},
		},
		{
			name:             "codex",
			adapter:          codex.New(),
			capabilitiesFile: "codex/capabilities.yaml",
			translateInputs: [][]byte{
				mustJSON(t, map[string]any{
					"hook_event_name": "PreToolUse",
					"session_id":      "conf-c1",
					"tool_name":       "shell",
				}),
				mustJSON(t, map[string]any{
					"hook_event_name": "SessionStart",
					"session_id":      "conf-c2",
				}),
				mustJSON(t, map[string]any{
					"hook_event_name": "PostToolUse",
					"session_id":      "conf-c3",
					"tool_name":       "shell",
				}),
			},
		},
		{
			name:             "vibe",
			adapter:          &vibe.Adapter{},
			capabilitiesFile: "vibe/capabilities.yaml",
			configFormat:     "toml",
			translateInputs: [][]byte{
				mustJSON(t, map[string]any{
					"event":      "ToolCallEvent",
					"session_id": "conf-v1",
					"tool_name":  "Write",
				}),
				mustJSON(t, map[string]any{
					"event":      "UserMessageEvent",
					"session_id": "conf-v2",
				}),
				mustJSON(t, map[string]any{
					"event":      "ToolResultEvent",
					"session_id": "conf-v3",
					"cwd":        "/tmp",
				}),
			},
		},
		{
			name:             "openhands",
			adapter:          &openhands.Adapter{},
			capabilitiesFile: "openhands/capabilities.yaml",
			translateInputs: [][]byte{
				mustJSON(t, map[string]any{
					"hook_event_name": "PreToolUse",
					"session_id":      "conf-oh1",
					"tool_name":       "Bash",
				}),
				mustJSON(t, map[string]any{
					"hook_event_name": "SessionStart",
					"session_id":      "conf-oh2",
				}),
				mustJSON(t, map[string]any{
					"hook_event_name": "PostToolUse",
					"session_id":      "conf-oh3",
					"tool_name":       "Write",
				}),
			},
		},
	}
}

// TestConformance_Detect verifies Detect works for each adapter.
// Skips if the CLI binary is not found on PATH.
func TestConformance_Detect(t *testing.T) {
	for _, ae := range allAdapters(t) {
		t.Run(ae.name, func(t *testing.T) {
			result, err := ae.adapter.Detect()
			if err != nil {
				t.Fatalf("Detect() error: %v", err)
			}
			if !result.Found {
				t.Skipf("%s CLI not installed, skipping", ae.name)
			}
			if result.Info.Name == "" {
				t.Error("Detect() returned Found=true but empty CLI name")
			}
		})
	}
}

// TestConformance_ConfigPaths verifies ConfigPaths returns non-empty,
// non-blank paths.
func TestConformance_ConfigPaths(t *testing.T) {
	for _, ae := range allAdapters(t) {
		t.Run(ae.name, func(t *testing.T) {
			paths := ae.adapter.ConfigPaths()
			if len(paths) == 0 {
				t.Fatal("ConfigPaths() returned empty slice")
			}
			for i, p := range paths {
				if p == "" {
					t.Errorf("ConfigPaths()[%d] is empty", i)
				}
			}
		})
	}
}

// TestConformance_MergeConfig_Empty verifies MergeConfig on nil input.
func TestConformance_MergeConfig_Empty(t *testing.T) {
	for _, ae := range allAdapters(t) {
		t.Run(ae.name, func(t *testing.T) {
			merged, err := ae.adapter.MergeConfig(nil)
			if err != nil {
				t.Fatalf("MergeConfig(nil) error: %v", err)
			}
			if len(merged) == 0 {
				t.Error("MergeConfig(nil) returned empty output")
			}
		})
	}
}

// TestConformance_MergeConfig_Existing verifies MergeConfig preserves
// existing config while adding nerv entries.
func TestConformance_MergeConfig_Existing(t *testing.T) {
	for _, ae := range allAdapters(t) {
		t.Run(ae.name, func(t *testing.T) {
			// TOML-based adapters get TOML input; JSON-based get JSON.
			var existing []byte
			if ae.configFormat == "toml" {
				existing = []byte("existing_key = \"preserved\"\n")
			} else {
				existing = []byte(`{"existing_key": "preserved"}`)
			}
			merged, err := ae.adapter.MergeConfig(existing)
			if err != nil {
				t.Fatalf("MergeConfig(existing) error: %v", err)
			}
			if len(merged) == 0 {
				t.Error("MergeConfig(existing) returned empty output")
			}
			// Verify the merged output is valid and preserves input.
			output := string(merged)
			if ae.configFormat == "toml" || ae.configFormat == "yaml" {
				// TOML/YAML: verify existing content preserved.
				if !strings.Contains(output, "existing_key") {
					t.Error(
						"merged non-JSON output lost existing_key",
					)
				}
			} else {
				var m map[string]any
				if err := json.Unmarshal(merged, &m); err != nil {
					t.Fatalf(
						"merged output is not valid JSON: %v",
						err,
					)
				}
				if m["existing_key"] != "preserved" {
					t.Error(
						"merged JSON output lost existing_key",
					)
				}
			}
		})
	}
}

// TestConformance_Translate verifies Translate round-trip for 3
// representative events per adapter.
func TestConformance_Translate(t *testing.T) {
	for _, ae := range allAdapters(t) {
		t.Run(ae.name, func(t *testing.T) {
			for i, input := range ae.translateInputs {
				translated, err := ae.adapter.Translate(input)
				if err != nil {
					t.Errorf(
						"Translate(input[%d]) error: %v",
						i, err,
					)
					continue
				}
				if len(translated) == 0 {
					t.Errorf(
						"Translate(input[%d]) returned empty",
						i,
					)
					continue
				}
				// Verify output is valid JSON.
				var m map[string]any
				if err := json.Unmarshal(translated, &m); err != nil {
					t.Errorf(
						"Translate(input[%d]) output not valid JSON: %v",
						i, err,
					)
				}
			}
		})
	}
}

// TestConformance_CapabilitiesMatch verifies that capabilities.yaml
// matches the adapter's Capabilities() return value.
func TestConformance_CapabilitiesMatch(t *testing.T) {
	for _, ae := range allAdapters(t) {
		t.Run(ae.name, func(t *testing.T) {
			capPath := filepath.Join(
				".", ae.capabilitiesFile,
			)
			data, err := os.ReadFile(capPath)
			if err != nil {
				t.Skipf(
					"capabilities.yaml not found at %s: %v",
					capPath, err,
				)
			}

			var doc capDoc
			if err := yaml.Unmarshal(data, &doc); err != nil {
				t.Fatalf("parse capabilities.yaml: %v", err)
			}

			caps := ae.adapter.Capabilities()

			// Verify PreToolUse capability matches.
			hasPreTool := hasEvent(doc.Native, "PreToolUse") ||
				hasEvent(doc.Native, "BeforeTool") ||
				hasEvent(doc.Native, "tool.execute.before") ||
				hasSynthesized(doc, "PreToolUse")
			if hasPreTool != caps.PreToolUse {
				t.Errorf(
					"PreToolUse: yaml=%v, code=%v",
					hasPreTool, caps.PreToolUse,
				)
			}

			// Verify PostToolUse capability matches.
			hasPostTool := hasEvent(doc.Native, "PostToolUse") ||
				hasEvent(doc.Native, "AfterTool") ||
				hasEvent(doc.Native, "tool.execute.after") ||
				hasSynthesized(doc, "PostToolUse")
			if hasPostTool != caps.PostToolUse {
				t.Errorf(
					"PostToolUse: yaml=%v, code=%v",
					hasPostTool, caps.PostToolUse,
				)
			}

			// Verify session events.
			hasPreSession := hasEvent(doc.Native, "SessionStart") ||
				hasEvent(doc.Native, "session.created") ||
				hasNativeMapping(doc.Native, "SessionStart")
			if hasPreSession != caps.PreSession {
				t.Errorf(
					"PreSession: yaml=%v, code=%v",
					hasPreSession, caps.PreSession,
				)
			}
		})
	}
}

// --- helpers ---

// capDoc is a minimal parse of capabilities.yaml.
type capDoc struct {
	CLI         string      `yaml:"cli"`
	Native      interface{} `yaml:"native"`
	Synthesized interface{} `yaml:"synthesized"`
	Unsupported []string    `yaml:"unsupported"`
}

// hasEvent checks if an event name appears in the native list.
// Handles both simple string lists and structured mapping lists.
func hasEvent(native interface{}, name string) bool {
	switch v := native.(type) {
	case []interface{}:
		for _, item := range v {
			switch entry := item.(type) {
			case string:
				if entry == name {
					return true
				}
			case map[string]interface{}:
				// Check both direct names and target mappings.
				for _, val := range entry {
					if s, ok := val.(string); ok && s == name {
						return true
					}
				}
			}
		}
	}
	return false
}

// hasNativeMapping checks for mapped native events (e.g. gemini format).
func hasNativeMapping(native interface{}, target string) bool {
	switch v := native.(type) {
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["nerv"].(string); ok && t == target {
					return true
				}
				if t, ok := m["target"].(string); ok && t == target {
					return true
				}
			}
		}
	}
	return false
}

// hasSynthesized checks if an event appears in the synthesized section.
func hasSynthesized(doc capDoc, name string) bool {
	switch v := doc.Synthesized.(type) {
	case map[string]interface{}:
		_, ok := v[name]
		return ok
	case []interface{}:
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["nerv"].(string); ok && t == name {
					return true
				}
				if t, ok := m["target"].(string); ok && t == name {
					return true
				}
			}
		}
	}
	return false
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustJSON: %v", err)
	}
	return b
}
