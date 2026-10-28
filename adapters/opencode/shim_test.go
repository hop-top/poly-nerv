package opencode

import (
	"strings"
	"testing"
)

func TestGeneratePlugin_Contains(t *testing.T) {
	src, err := GeneratePlugin("/usr/local/bin/nervd")
	if err != nil {
		t.Fatalf("GeneratePlugin() error: %v", err)
	}

	required := []string{
		`import { execFileSync } from "child_process"`,
		`NERVD_PATH = "/usr/local/bin/nervd"`,
		`"session.created"`,
		`"tool.execute.before"`,
		`"tool.execute.after"`,
		`"file.edited"`,
		`"permission.asked"`,
		`"permission.replied"`,
		`"session.deleted"`,
		`"session.idle"`,
		`"session.error"`,
		`"session.compacted"`,
		`"tui.prompt.append"`,
		`"todo.updated"`,
		`handleBlocking(event)`,
		`handlePassive(event)`,
		`dispatch(event)`,
		`throw new Error`,
		`export default`,
	}

	for _, s := range required {
		if !strings.Contains(src, s) {
			t.Errorf("output missing %q", s)
		}
	}
}

func TestGeneratePlugin_ValidTS(t *testing.T) {
	src, err := GeneratePlugin("/usr/local/bin/nervd")
	if err != nil {
		t.Fatalf("GeneratePlugin() error: %v", err)
	}

	// Basic syntax check: balanced braces.
	open := strings.Count(src, "{")
	close := strings.Count(src, "}")
	if open != close {
		t.Errorf("unbalanced braces: %d open, %d close", open, close)
	}

	// Balanced parens.
	openP := strings.Count(src, "(")
	closeP := strings.Count(src, ")")
	if openP != closeP {
		t.Errorf("unbalanced parens: %d open, %d close", openP, closeP)
	}
}

func TestGeneratePlugin_EmptyPath(t *testing.T) {
	_, err := GeneratePlugin("")
	if err == nil {
		t.Error("expected error for empty nervdPath")
	}
}

func TestGeneratePlugin_WindowsPath(t *testing.T) {
	src, err := GeneratePlugin(`C:\Program Files\nerv\nervd.exe`)
	if err != nil {
		t.Fatalf("GeneratePlugin() error: %v", err)
	}
	if !strings.Contains(src, `C:\\Program Files\\nerv\\nervd.exe`) {
		t.Error("Windows path not properly escaped")
	}
}
