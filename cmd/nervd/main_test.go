package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/dispatch"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// TestInitActiveProfile_AspEnv verifies the daemon-init hook returns
// the live aps profile when APS_PROFILE_ID is set, with no warning
// emitted. Locks in fail-open semantics for the happy path.
func TestInitActiveProfile_AspEnv(t *testing.T) {
	t.Setenv("APS_PROFILE_ID", "test-profile")

	var buf bytes.Buffer
	got := initActiveProfile(&buf)

	if got != "test-profile" {
		t.Errorf("initActiveProfile() = %q, want %q", got, "test-profile")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no warning when profile resolved, got: %q", buf.String())
	}
}

// TestInitActiveProfile_NoAspEnv verifies fail-open: no env → empty
// id + warning to stderr. The daemon must still be runnable; main()
// continues regardless of the return value.
func TestInitActiveProfile_NoAspEnv(t *testing.T) {
	t.Setenv("APS_PROFILE_ID", "")

	var buf bytes.Buffer
	got := initActiveProfile(&buf)

	if got != "" {
		t.Errorf("initActiveProfile() = %q, want empty", got)
	}
	if !strings.Contains(buf.String(), "no active aps profile") {
		t.Errorf("expected warning about missing profile, got: %q", buf.String())
	}
}

// TestDecodeStateless_ClaudeNative verifies stdin carrying Claude's
// native hook JSON (hook_event_name) is translated to an envelope and
// flagged as Claude dialect.
func TestDecodeStateless_ClaudeNative(t *testing.T) {
	raw := []byte(`{
		"hook_event_name": "WorktreeCreate",
		"session_id": "sess-1",
		"cwd": "/repo"
	}`)

	env, claudeEvent, err := decodeStateless(raw)
	if err != nil {
		t.Fatalf("decodeStateless() error: %v", err)
	}
	if claudeEvent != "WorktreeCreate" {
		t.Errorf("claudeEvent = %q, want %q", claudeEvent, "WorktreeCreate")
	}
	if env.Event != "WorktreeCreate" {
		t.Errorf("env.Event = %q, want %q", env.Event, "WorktreeCreate")
	}
	if env.Session.ID != "sess-1" {
		t.Errorf("Session.ID = %q, want %q", env.Session.ID, "sess-1")
	}
}

// TestDecodeStateless_Envelope verifies nerv-envelope input still
// decodes as before, with no Claude dialect flag.
func TestDecodeStateless_Envelope(t *testing.T) {
	src := &envelope.Envelope{
		NervVersion: "1.0-draft",
		Event:       "PreToolUse",
		EventID:     "evt-1",
	}
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	env, claudeEvent, err := decodeStateless(raw)
	if err != nil {
		t.Fatalf("decodeStateless() error: %v", err)
	}
	if claudeEvent != "" {
		t.Errorf("claudeEvent = %q, want empty", claudeEvent)
	}
	if env.Event != "PreToolUse" {
		t.Errorf("env.Event = %q, want %q", env.Event, "PreToolUse")
	}
}

func TestDecodeStateless_InvalidJSON(t *testing.T) {
	if _, _, err := decodeStateless([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestRespondClaude_WorktreeCreate_SilentStdout locks in the fix for
// worktree creation: Claude reads a WorktreeCreate hook's stdout as a
// directory path, so nervd must write nothing and exit 0.
func TestRespondClaude_WorktreeCreate_SilentStdout(t *testing.T) {
	var out, errw bytes.Buffer

	code := respondClaude(&out, &errw, "WorktreeCreate",
		merger.Decision{Action: merger.Allow})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
}

// TestStatelessPipeline_WorktreeCreate_EmptyStdout is the regression
// test for worktree creation breaking under nervd: Claude invokes the
// WorktreeCreate hook with native JSON and treats any stdout as the
// worktree directory path, so the full decode→dispatch→respond
// pipeline must produce zero stdout bytes and exit 0. Before the fix,
// nervd echoed the merged decision JSON and every isolated-worktree
// creation failed with "returned a path that is not a directory".
func TestStatelessPipeline_WorktreeCreate_EmptyStdout(t *testing.T) {
	raw := []byte(`{
		"hook_event_name": "WorktreeCreate",
		"session_id": "sess-1",
		"cwd": "/repo"
	}`)

	env, claudeEvent, err := decodeStateless(raw)
	if err != nil {
		t.Fatalf("decodeStateless() error: %v", err)
	}
	if claudeEvent != "WorktreeCreate" {
		t.Fatalf("claudeEvent = %q, want WorktreeCreate", claudeEvent)
	}

	cc := config.Compile(&config.HooksConfig{})
	d := dispatch.New(cc, nil, dispatch.NewMatcher(), dispatch.NewRunner())
	res, err := d.Dispatch(env)
	if err != nil {
		t.Fatalf("Dispatch() error: %v", err)
	}

	var out, errw bytes.Buffer
	code := respondClaude(&out, &errw, claudeEvent, res.Decision)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty (Claude reads it as a path)", out.String())
	}
}

func TestRespondClaude_Allow(t *testing.T) {
	var out, errw bytes.Buffer

	code := respondClaude(&out, &errw, "PreToolUse",
		merger.Decision{Action: merger.Allow})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if out.String() != "{}" {
		t.Errorf("stdout = %q, want %q", out.String(), "{}")
	}
}

func TestRespondClaude_Block(t *testing.T) {
	var out, errw bytes.Buffer

	code := respondClaude(&out, &errw, "PreToolUse",
		merger.Decision{Action: merger.Block, Message: "nope"})

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if _, ok := result["hookSpecificOutput"]; !ok {
		t.Errorf("stdout missing hookSpecificOutput: %q", out.String())
	}
}

func TestRespondClaude_Warn_StderrOnly(t *testing.T) {
	var out, errw bytes.Buffer

	code := respondClaude(&out, &errw, "PostToolUse",
		merger.Decision{Action: merger.Warn, Message: "careful"})

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if out.String() != "{}" {
		t.Errorf("stdout = %q, want %q", out.String(), "{}")
	}
	if !strings.Contains(errw.String(), "careful") {
		t.Errorf("stderr = %q, want warn message", errw.String())
	}
}
