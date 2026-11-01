//go:build !windows

package opencode

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBridgePlugin_UnderBun renders the plugin, runs it under bun
// against a stub nervd, and drives the hooks the way OpenCode does:
// session start through the generic "event" hook, then the system
// transform and tool.execute.after context channels.
func TestBridgePlugin_UnderBun(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun not installed")
	}
	dir := t.TempDir()

	// Stub nervd: records argv + stdin, answers per event type with the
	// DecisionJSON wire shape.
	stub := filepath.Join(dir, "nervd")
	stubSrc := `#!/bin/sh
echo "$@" >> "` + dir + `/argv.log"
in=$(cat)
echo "$in" >> "` + dir + `/stdin.log"
case "$in" in
  *session.created*) echo '{"action":"allow","context":"SESSION-CTX"}';;
  *tool.execute.after*) echo '{"action":"allow","context":"TOOL-CTX"}';;
  *) echo '{"action":"allow"}';;
esac
`
	if err := os.WriteFile(stub, []byte(stubSrc), 0o700); err != nil {
		t.Fatal(err)
	}

	src, err := GeneratePlugin(stub)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.ts"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	driver := `import plugin from "./plugin.ts";
const h: any = (plugin as any)({});
await h.event({ event: { type: "session.created", properties: { info: { id: "ses_1" } } } });
const sys = { system: ["base"] };
await h["experimental.chat.system.transform"]({ sessionID: "ses_1" }, sys);
const other = { system: ["base"] };
await h["experimental.chat.system.transform"]({ sessionID: "ses_2" }, other);
const tool = { output: "ls out" };
await h["tool.execute.after"]({ tool: "bash", sessionID: "ses_1", args: {} }, tool);
await h["tool.execute.before"]({ tool: "bash", sessionID: "ses_1" }, { args: { command: "ls" } });
console.log(JSON.stringify({ sys, other, tool }));
`
	if err := os.WriteFile(filepath.Join(dir, "driver.ts"), []byte(driver), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bun, "run", "driver.ts")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bun: %v\n%s", err, out)
	}
	t.Logf("bun output: %s", out)

	var got struct {
		Sys   struct{ System []string } `json:"sys"`
		Other struct{ System []string } `json:"other"`
		Tool  struct{ Output string }   `json:"tool"`
	}
	if err := json.Unmarshal(out[strings.Index(string(out), "{"):], &got); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if len(got.Sys.System) != 2 || got.Sys.System[1] != "SESSION-CTX" {
		t.Errorf("session context not applied: %v", got.Sys.System)
	}
	if len(got.Other.System) != 1 {
		t.Errorf("context leaked to another session: %v", got.Other.System)
	}
	if got.Tool.Output != "ls out\nTOOL-CTX" {
		t.Errorf("tool context not appended: %q", got.Tool.Output)
	}

	argv, _ := os.ReadFile(filepath.Join(dir, "argv.log"))
	if lines := strings.Split(strings.TrimSpace(string(argv)), "\n"); len(lines) != 3 {
		t.Fatalf("want 3 nervd calls, got %q", argv)
	} else {
		for _, l := range lines {
			if l != "--adapter opencode" {
				t.Errorf("nervd argv = %q, want --adapter opencode", l)
			}
		}
	}
	stdin, _ := os.ReadFile(filepath.Join(dir, "stdin.log"))
	if !strings.Contains(string(stdin), `"type":"tool.execute.before"`) {
		t.Errorf("tool.execute.before sent without type: %s", stdin)
	}
}
