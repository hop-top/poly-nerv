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
  *'rm -rf'*) echo '{"action":"block","message":"nerv: rm denied"}';;
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

// Bus events arrive only through the generic event hook, shaped like
// the runtime publishes them: { id, type, properties }.
const bus = (type: string, properties: any) =>
  h.event({ event: { id: "evt_" + type, type, properties } });

await bus("session.created", { sessionID: "ses_1", info: { id: "ses_1" } });
const sys = { system: ["base"] };
await h["experimental.chat.system.transform"]({ sessionID: "ses_1" }, sys);
const other = { system: ["base"] };
await h["experimental.chat.system.transform"]({ sessionID: "ses_2" }, other);
const tool = { output: "ls out" };
await h["tool.execute.after"]({ tool: "bash", sessionID: "ses_1", args: {} }, tool);
await h["tool.execute.before"]({ tool: "bash", sessionID: "ses_1", callID: "c1" }, { args: { command: "ls" } });

await bus("session.idle", { sessionID: "ses_1" });
await bus("session.error", { sessionID: "ses_1", error: { name: "UnknownError", data: { message: "boom" } } });
await bus("session.compacted", { sessionID: "ses_1" });
await bus("file.edited", { file: "/tmp/a.go" });
await bus("permission.asked", { id: "per_1", sessionID: "ses_1", permission: "bash", patterns: ["rm *"], metadata: {}, always: [] });
await bus("permission.replied", { sessionID: "ses_1", requestID: "per_1", reply: "reject" });
await bus("permission.replied", { sessionID: "ses_1", requestID: "per_2", reply: "once" });
await bus("tui.prompt.append", { text: "hello" });
await bus("todo.updated", { sessionID: "ses_1", todos: [{ id: "t1", content: "a", status: "completed", priority: "low" }, { id: "t2", content: "b", status: "pending", priority: "low" }] });
await bus("todo.updated", { sessionID: "ses_1", todos: [{ id: "t1", content: "a", status: "completed", priority: "low" }, { id: "t2", content: "b", status: "pending", priority: "low" }] });
await bus("message.updated", { info: {} });
await bus("session.deleted", { sessionID: "ses_1", info: { id: "ses_1" } });

let blocked = "";
try {
  await h["tool.execute.before"]({ tool: "bash", sessionID: "ses_1", callID: "c2" }, { args: { command: "rm -rf /" } });
} catch (e: any) {
  blocked = String(e.message);
}

// Dead top-level keys: v1 never calls them.
const keys = Object.keys(h).sort();
console.log(JSON.stringify({ sys, other, tool, blocked, keys }));
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
		Sys     struct{ System []string } `json:"sys"`
		Other   struct{ System []string } `json:"other"`
		Tool    struct{ Output string }   `json:"tool"`
		Blocked string                    `json:"blocked"`
		Keys    []string                  `json:"keys"`
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

	if got.Blocked != "nerv: rm denied" {
		t.Errorf("tool.execute.before did not throw block message: %q", got.Blocked)
	}
	wantKeys := []string{
		"event", "experimental.chat.system.transform",
		"tool.execute.after", "tool.execute.before",
	}
	if strings.Join(got.Keys, ",") != strings.Join(wantKeys, ",") {
		t.Errorf("hook keys = %v, want only real v1 hooks %v", got.Keys, wantKeys)
	}

	argv, _ := os.ReadFile(filepath.Join(dir, "argv.log"))
	lines := strings.Split(strings.TrimSpace(string(argv)), "\n")
	for _, l := range lines {
		if l != "--adapter opencode" {
			t.Errorf("nervd argv = %q, want --adapter opencode", l)
		}
	}

	// One envelope per routed event, in order. Skipped: permission.replied
	// with reply "once" (only reject maps to PermissionDenied), the second
	// identical todo.updated (completed ids already reported), and the
	// unmapped message.updated.
	var types []string
	var sent []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(string(mustRead(t, filepath.Join(dir, "stdin.log")))), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("stdin line %q: %v", l, err)
		}
		types = append(types, m["type"].(string))
		sent = append(sent, m)
	}
	wantTypes := []string{
		"session.created", "tool.execute.after", "tool.execute.before",
		"session.idle", "session.error", "session.compacted", "file.edited",
		"permission.asked", "permission.replied", "tui.prompt.append",
		"todo.updated", "session.deleted", "tool.execute.before",
	}
	if strings.Join(types, ",") != strings.Join(wantTypes, ",") {
		t.Fatalf("event sequence\n got %v\nwant %v", types, wantTypes)
	}
	by := func(typ string) map[string]any {
		for _, m := range sent {
			if m["type"] == typ {
				return m
			}
		}
		return nil
	}
	checks := []struct{ typ, field, want string }{
		{"session.created", "session_id", "ses_1"},
		{"session.idle", "session_id", "ses_1"},
		{"session.error", "error", "boom"},
		{"session.compacted", "session_id", "ses_1"},
		{"file.edited", "path", "/tmp/a.go"},
		{"permission.asked", "session_id", "ses_1"},
		{"permission.replied", "decision", "deny"},
		{"tui.prompt.append", "input", "hello"},
		{"session.deleted", "session_id", "ses_1"},
	}
	for _, c := range checks {
		m := by(c.typ)
		if m == nil || m[c.field] != c.want {
			t.Errorf("%s: %s = %v, want %q", c.typ, c.field, m[c.field], c.want)
		}
	}
	if m := by("todo.updated"); m == nil || m["session_id"] != "ses_1" {
		t.Errorf("todo.updated payload: %v", m)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// runBun renders plugin src plus driver into a temp dir with a stub nervd
// whose body is stubCase (a sh `case "$in" in ... esac` arm list), runs the
// driver under bun and returns its last JSON line.
func runBun(t *testing.T, gen func(string) (string, error), stubCase, driver string) map[string]any {
	t.Helper()
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun not installed")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "nervd")
	stubSrc := "#!/bin/sh\nin=$(cat)\necho \"$in\" >> \"" + dir + "/stdin.log\"\ncase \"$in\" in\n" +
		stubCase + "\n  *) echo '{\"action\":\"allow\"}';;\nesac\n"
	if err := os.WriteFile(stub, []byte(stubSrc), 0o700); err != nil {
		t.Fatal(err)
	}
	src, err := gen(stub)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.ts"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
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
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var got map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &got); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	return got
}

// A rewrite decision on tool.execute.before replaces the args object the
// tool will run with, in place; v1 does not rename args, so the rewrite is
// applied verbatim. Rewrite on any other event is ignored with a warning.
func TestBridgePlugin_Rewrite(t *testing.T) {
	stubCase := `  *'"command":"ls"'*) echo '{"action":"rewrite","message":"w","rewrite":{"command":"echo rewritten","description":"d"}}';;
  *'"command":"pathy"'*) echo '{"action":"rewrite","rewrite":{"filePath":"/new","path":"/p","keep":1}}';;
  *'"command":"bad"'*) echo '{"action":"rewrite","rewrite":"nope"}';;
  *'"command":"empty"'*) echo '{"action":"rewrite"}';;
  *tool.execute.after*) echo '{"action":"rewrite","rewrite":{"x":1}}';;`
	driver := `import plugin from "./plugin.ts";
const h: any = (plugin as any)({});
const warns: string[] = [];
console.warn = (...a: any[]) => { warns.push(a.join(" ")); };

const args: any = { command: "ls", timeout: 5, extra: true };
const ref = args;
await h["tool.execute.before"]({ tool: "bash", sessionID: "s" }, { args });

const pathy: any = { command: "pathy", filePath: "/old" };
await h["tool.execute.before"]({ tool: "write", sessionID: "s" }, { args: pathy });

const bad: any = { command: "bad" };
await h["tool.execute.before"]({ tool: "bash", sessionID: "s" }, { args: bad });
const empty: any = { command: "empty" };
await h["tool.execute.before"]({ tool: "bash", sessionID: "s" }, { args: empty });

const afterArgs: any = { command: "zzz" };
const out = { output: "o" };
await h["tool.execute.after"]({ tool: "bash", sessionID: "s", args: afterArgs }, out);

console.log(JSON.stringify({ args, same: ref === args, pathy, bad, empty, afterArgs, out, warns }));
`
	got := runBun(t, GeneratePlugin, stubCase, driver)
	eq := func(name string, v any, want string) {
		t.Helper()
		b, _ := json.Marshal(v)
		if string(b) != want {
			t.Errorf("%s = %s, want %s", name, b, want)
		}
	}
	eq("args", got["args"], `{"command":"echo rewritten","description":"d"}`)
	eq("same ref", got["same"], `true`)
	eq("pathy (v1: no key mapping)", got["pathy"], `{"filePath":"/new","keep":1,"path":"/p"}`)
	eq("bad (non-object rewrite ignored)", got["bad"], `{"command":"bad"}`)
	eq("empty (no payload ignored)", got["empty"], `{"command":"empty"}`)
	eq("afterArgs", got["afterArgs"], `{"command":"zzz"}`)
	eq("after out", got["out"], `{"output":"o"}`)
	warns, _ := json.Marshal(got["warns"])
	for _, w := range []string{"warning: w", "rewrite decision ignored (needs an object", "only tool.execute.before supports rewrite"} {
		if !strings.Contains(string(warns), w) {
			t.Errorf("missing warning %q in %s", w, warns)
		}
	}
}
