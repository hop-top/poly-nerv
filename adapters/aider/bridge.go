package aider

import (
	"encoding/json"
	"fmt"
	"strings"

	"hop.top/nerv/internal/merger"
)

// Aider has no hook protocol. Its only extension points are the
// lint-cmd and test-cmd shell commands, which Aider runs after it has
// applied an edit (aider/coders/base_coder.py: lint_edited, cmd_test).
// lint-cmd receives the edited file as a trailing shell-quoted argv
// word (aider/linter.py: run_cmd); test-cmd receives no arguments.
// Neither gets JSON on stdin, so the entrypoint is argv:
//
//	nervd --adapter aider lint <file>...
//	nervd --adapter aider test

// Bridge verbs accepted as the first positional argument.
const (
	VerbLint = "lint"
	VerbTest = "test"
)

// BridgeEvent builds the synthesized event JSON for one lint-cmd or
// test-cmd invocation. args is the argv after the flags: a verb, then
// (lint only) the files Aider appended. lint maps to FileChanged and
// test to PostToolUse; both fire after the edit is on disk, so neither
// can prevent it.
func BridgeEvent(args []string, cwd string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("aider: missing verb (want %s or %s)",
			VerbLint, VerbTest)
	}
	m := map[string]any{"cwd": cwd}
	switch args[0] {
	case VerbLint:
		files := args[1:]
		m["hook_event_name"] = "FileChanged"
		m["source"] = "lint-cmd"
		m["change_type"] = "modified"
		if len(files) > 0 {
			m["file_path"] = files[0]
			m["files"] = files
		}
	case VerbTest:
		m["hook_event_name"] = "PostToolUse"
		m["source"] = "test-cmd"
		m["tool_name"] = "test-cmd"
	default:
		return nil, fmt.Errorf("aider: unknown verb %q (want %s or %s)",
			args[0], VerbLint, VerbTest)
	}
	return json.Marshal(m)
}

// FromEventDecision renders a decision as the stdout bytes and exit
// code Aider acts on. Aider feeds lint/test output to the model only
// on a non-zero exit, as an error-repair prompt, so:
//
//	block   -> message on stdout, exit 1
//	allow   -> nothing, exit 0
//	warn    -> nothing on stdout, exit 0 (nervd logs the text to stderr;
//	           Aider shows it to the user but never to the model)
//	rewrite -> dropped, exit 0 (there is no input to rewrite)
//
// context is dropped: a non-zero exit is the only way to reach the
// model, and that would present context as a failure.
func FromEventDecision(d merger.Decision, _ string) ([]byte, int, error) {
	if d.Action != merger.Block {
		return nil, 0, nil
	}
	msg := strings.TrimSpace(d.Message)
	if msg == "" {
		msg = "blocked by nerv"
	}
	return []byte(msg), 1, nil
}
