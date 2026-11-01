package goose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/nerv/internal/merger"
)

const wantNervCommand = "nervd --adapter goose"

// hookCommands returns event -> commands from a hooks.json document.
func hookCommands(t *testing.T, doc []byte) map[string][]string {
	t.Helper()
	var parsed struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(doc, &parsed); err != nil {
		t.Fatalf("merged config is not valid hooks.json: %v\n%s", err, doc)
	}
	out := map[string][]string{}
	for ev, rules := range parsed.Hooks {
		for _, r := range rules {
			for _, h := range r.Hooks {
				if h.Type != "command" {
					t.Fatalf("%s: type %q, want command", ev, h.Type)
				}
				out[ev] = append(out[ev], h.Command)
			}
		}
	}
	return out
}

var wantEvents = []string{
	"AfterFileEdit", "PostToolUse", "PostToolUseFailure", "PreToolUse",
	"SessionEnd", "SessionStart", "Stop", "UserPromptSubmit",
}

func TestConfigPaths_PluginHooksFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got := (&Adapter{}).ConfigPaths()
	want := filepath.Join(home, ".agents", "plugins", "nerv", "hooks", "hooks.json")
	if len(got) == 0 || got[0] != want {
		t.Fatalf("ConfigPaths()[0] = %v, want %s", got, want)
	}
}

func TestMergeConfig_Fresh(t *testing.T) {
	out, err := (&Adapter{}).MergeConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	cmds := hookCommands(t, out)
	if len(cmds) != len(wantEvents) {
		t.Fatalf("events = %v, want %v", cmds, wantEvents)
	}
	for _, ev := range wantEvents {
		if len(cmds[ev]) != 1 || cmds[ev][0] != wantNervCommand {
			t.Errorf("%s commands = %v, want [%s]", ev, cmds[ev], wantNervCommand)
		}
	}
	if strings.Contains(string(out), "nerv-bridge") ||
		strings.Contains(string(out), "dispatch") {
		t.Fatalf("legacy bridge text present:\n%s", out)
	}
}

func TestMergeConfig_Idempotent(t *testing.T) {
	a := &Adapter{}
	first, _ := a.MergeConfig(nil)
	second, err := a.MergeConfig(first)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("not idempotent:\n%s\n---\n%s", first, second)
	}
}

// Legacy entries (nerv dispatch goose, bare nervd) are rewritten in
// place; unrelated hooks and top-level keys survive; no duplicates.
func TestMergeConfig_UpgradesLegacyInPlace(t *testing.T) {
	legacy := []byte(`{
  "extra": 1,
  "hooks": {
    "PreToolUse": [
      {"matcher": "^shell$", "hooks": [
        {"type": "command", "command": "nerv dispatch goose", "timeout": 5},
        {"type": "command", "command": "/opt/mine.sh"}
      ]}
    ],
    "Stop": [{"hooks": [{"type": "command", "command": "nervd"}]}],
    "PostToolUse": [{"hooks": [{"type": "command", "command": "/opt/other.sh"}]}]
  }
}`)
	out, err := (&Adapter{}).MergeConfig(legacy)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "nerv dispatch goose") || strings.Contains(s, `"command": "nervd"`) {
		t.Fatalf("legacy command left:\n%s", s)
	}
	cmds := hookCommands(t, out)
	if got := cmds["PreToolUse"]; len(got) != 2 || got[0] != wantNervCommand || got[1] != "/opt/mine.sh" {
		t.Errorf("PreToolUse = %v", got)
	}
	if got := cmds["Stop"]; len(got) != 1 || got[0] != wantNervCommand {
		t.Errorf("Stop = %v", got)
	}
	if got := cmds["PostToolUse"]; len(got) != 2 || got[0] != "/opt/other.sh" || got[1] != wantNervCommand {
		t.Errorf("PostToolUse = %v (user hook kept, nerv appended)", got)
	}
	if !strings.Contains(s, `"extra": 1`) || !strings.Contains(s, `"matcher": "^shell$"`) ||
		!strings.Contains(s, `"timeout": 5`) {
		t.Errorf("unrelated fields lost:\n%s", s)
	}
	again, _ := (&Adapter{}).MergeConfig(out)
	if string(again) != s {
		t.Fatalf("upgrade not idempotent")
	}
}

func TestMergeConfig_InvalidJSON(t *testing.T) {
	if _, err := (&Adapter{}).MergeConfig([]byte("extensions:\n  x: 1\n")); err == nil {
		t.Fatal("want error for non-JSON existing config")
	}
}

// Payloads captured from goose 1.55.0 (hooks.json command, stdin).
func TestToEnvelope_RealHookPayloads(t *testing.T) {
	cases := []struct {
		name, in, event string
	}{
		{"session_start", `{"event":"SessionStart","session_id":"s1","matcher_context":null,"working_dir":"/w"}`, "SessionStart"},
		{"pre_tool", `{"event":"PreToolUse","session_id":"s1","matcher_context":"shell","tool_call_id":"c1","tool_name":"shell","tool_input":{"command":"ls"},"working_dir":"/w"}`, "PreToolUse"},
		{"post_tool", `{"event":"PostToolUse","session_id":"s1","tool_name":"shell","working_dir":"/w"}`, "PostToolUse"},
		{"post_fail", `{"event":"PostToolUseFailure","session_id":"s1","tool_name":"edit","working_dir":"/w"}`, "PostToolUseFailure"},
		{"stop", `{"event":"Stop","session_id":"s1","last_assistant_message":"done","working_dir":"/w"}`, "Stop"},
		{"end", `{"event":"SessionEnd","session_id":"s1"}`, "SessionEnd"},
		{"prompt", `{"event":"UserPromptSubmit","session_id":"s1","message":"hi"}`, "UserPromptSubmit"},
		{"file_edit", `{"event":"AfterFileEdit","session_id":"s1","matcher_context":"/w/a.rs"}`, "FileChanged"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal([]byte(tc.in), &m); err != nil {
				t.Fatal(err)
			}
			env, err := ToEnvelope(m)
			if err != nil {
				t.Fatal(err)
			}
			if env.Event != tc.event {
				t.Errorf("event = %s, want %s", env.Event, tc.event)
			}
			if env.Synthesized {
				t.Errorf("hook events are native, got synthesized")
			}
			if env.Session.ID != "s1" {
				t.Errorf("session id = %q", env.Session.ID)
			}
			if wd, _ := m["working_dir"].(string); wd != "" && env.Session.CWD != wd {
				t.Errorf("cwd = %q, want %q", env.Session.CWD, wd)
			}
		})
	}
}

func TestToEnvelope_RealHookUnknownEvent(t *testing.T) {
	_, err := ToEnvelope(map[string]any{"event": "PreToolUseResult"})
	if err == nil {
		t.Fatal("PreToolUseResult is observation-only and not mapped; want error")
	}
}

func TestFromEventDecision(t *testing.T) {
	cases := []struct {
		name   string
		d      merger.Decision
		event  string
		want   string // JSON, "" = empty stdout
		wantRC int
	}{
		{"block_pre", merger.Decision{Action: merger.Block, Message: "no rm"}, "PreToolUse",
			`{"decision":"block","reason":"no rm"}`, 0},
		{"block_stop", merger.Decision{Action: merger.Block, Message: "run tests"}, "Stop",
			`{"decision":"block","reason":"run tests"}`, 0},
		{"block_default_reason", merger.Decision{Action: merger.Block}, "PreToolUse",
			`{"decision":"block","reason":"blocked by nerv"}`, 0},
		{"block_ignored_event", merger.Decision{Action: merger.Block, Message: "x"}, "PostToolUse", ``, 0},
		{"allow", merger.Decision{Action: merger.Allow}, "PreToolUse", ``, 0},
		{"allow_context_dropped", merger.Decision{Action: merger.Allow, Context: "ctx"}, "SessionStart", ``, 0},
		{"warn", merger.Decision{Action: merger.Warn, Message: "careful"}, "PreToolUse", ``, 0},
		{"rewrite_dropped", merger.Decision{Action: merger.Rewrite}, "PreToolUse", ``, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, rc, err := FromEventDecision(tc.d, tc.event)
			if err != nil {
				t.Fatal(err)
			}
			if rc != tc.wantRC {
				t.Errorf("rc = %d, want %d", rc, tc.wantRC)
			}
			if tc.want == "" {
				if m != nil {
					t.Fatalf("want no stdout, got %v", m)
				}
				return
			}
			b, _ := json.Marshal(m)
			if string(b) != tc.want {
				t.Fatalf("got %s want %s", b, tc.want)
			}
		})
	}
}

func TestDetect_LegacyConfigFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if err := os.MkdirAll(filepath.Join(home, ".config", "goose"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "goose", "config.yaml"), []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r, _ := (&Adapter{}).Detect()
	if !r.Found {
		t.Fatal("goose config dir should signal presence")
	}
}
