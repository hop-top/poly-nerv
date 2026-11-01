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

// TestBridgePluginV2_UnderBun renders the v2 plugin, runs it under bun
// against a stub nervd and a fake plugin context, and drives the hooks
// the way OpenCode 2.x does: bus events through ctx.event.subscribe,
// "context" for session start, execute.before/after for tools.
func TestBridgePluginV2_UnderBun(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun not installed")
	}
	dir := t.TempDir()

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
	src, err := GeneratePluginV2(stub)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.ts"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	driver := `import plugin from "./plugin.ts";

// Fake plugin context: records hook registrations, and feeds the
// event.subscribe iterator from a queue the driver pushes into.
const hooks: Record<string, Function> = {};
const queue: any[] = [];
let wake: (() => void) | null = null;
let subscribeOpts: any = null;
const ctx: any = {
  location: { directory: "/proj" },
  options: {},
  tool: { hook: async (n: string, cb: Function) => { hooks["tool." + n] = cb; return { dispose: async () => {} }; } },
  session: { hook: async (n: string, cb: Function) => { hooks["session." + n] = cb; return { dispose: async () => {} }; } },
  event: {
    subscribe(opts: any) {
      subscribeOpts = opts;
      return {
        [Symbol.asyncIterator]() {
          return {
            async next() {
              while (queue.length === 0) {
                if (opts.signal?.aborted) return { done: true, value: undefined };
                await new Promise<void>((r) => { wake = r; opts.signal?.addEventListener("abort", () => r(), { once: true }); });
              }
              return { done: false, value: queue.shift() };
            },
            async return() { return { done: true, value: undefined }; },
          };
        },
      };
    },
  },
};

const def: any = plugin;
const cleanup = await def.setup(ctx);
const bus = async (type: string, data: any) => {
  queue.push({ id: "evt_" + type, type, data });
  wake?.();
  await new Promise((r) => setTimeout(r, 300));
};

// Session start comes from the first model request (no session.created seen).
const sys1 = { sessionID: "ses_1", system: [{ type: "text", text: "base" }] };
await hooks["session.context"](sys1);
const sys1again: any = { sessionID: "ses_1", system: [] };
await hooks["session.context"](sys1again); // later request: context again, no 2nd dispatch
const sys2 = { sessionID: "ses_2", system: [{ type: "text", text: "base" }] };
await bus("session.created", { sessionID: "ses_2" });
await hooks["session.context"](sys2);

await hooks["tool.execute.before"]({ tool: "shell", sessionID: "ses_1", id: "c1", input: { command: "ls" } });
const arrRes: any = { output: { exit: 0 }, content: [{ type: "text", text: "ls out" }] };
await hooks["tool.execute.after"]({ tool: "shell", sessionID: "ses_1", id: "c1", input: { command: "ls" }, status: "completed", result: arrRes });
const strRes: any = { content: "plain" };
await hooks["tool.execute.after"]({ tool: "read", sessionID: "ses_1", id: "c2", input: { path: "/a/CLAUDE.md" }, status: "completed", result: strRes });
const noRes: any = { output: 1 };
await hooks["tool.execute.after"]({ tool: "grep", sessionID: "ses_1", id: "c3", input: {}, status: "completed", result: noRes });
await hooks["tool.execute.after"]({ tool: "write", sessionID: "ses_1", id: "c4", input: { path: "/tmp/a.go", content: "x" }, status: "completed", result: { content: "ok" } });
await hooks["tool.execute.after"]({ tool: "patch", sessionID: "ses_1", id: "c5", input: {}, status: "completed", result: { output: { applied: [{ type: "update", resource: "b.go", target: "/proj/b.go" }] }, content: "ok" } });
const todos = [{ content: "a", status: "completed" }, { content: "b", status: "pending" }];
await hooks["tool.execute.after"]({ tool: "todowrite", sessionID: "ses_1", id: "c6", input: { todos }, status: "completed", result: { output: { todos }, content: "t" } });
await hooks["tool.execute.after"]({ tool: "todowrite", sessionID: "ses_1", id: "c7", input: { todos }, status: "completed", result: { output: { todos }, content: "t" } });
const errRes = { tool: "shell", sessionID: "ses_1", id: "c8", input: { command: "false" }, status: "error", error: { message: "exit 1" } };
await hooks["tool.execute.after"](errRes);

await bus("session.execution.succeeded", { sessionID: "ses_1" });
await bus("session.execution.interrupted", { sessionID: "ses_1", reason: "user" });
await bus("session.execution.interrupted", { sessionID: "ses_1", reason: "shutdown" });
await bus("session.execution.failed", { sessionID: "ses_1", error: { type: "api", message: "boom" } });
await bus("session.compaction.ended", { sessionID: "ses_1", reason: "auto" });
await bus("permission.asked", { id: "per_1", sessionID: "ses_1", action: "shell", resources: ["rm *"] });
await bus("permission.replied", { sessionID: "ses_1", requestID: "per_1", reply: "reject" });
await bus("permission.replied", { sessionID: "ses_1", requestID: "per_2", reply: "once" });
await bus("tui.prompt.append", { text: "hello" });
await bus("session.deleted", { sessionID: "ses_2" });

let blocked = "";
try {
  await hooks["tool.execute.before"]({ tool: "shell", sessionID: "ses_1", id: "c9", input: { command: "rm -rf /" } });
} catch (e: any) { blocked = String(e.message); }

await cleanup();
console.log(JSON.stringify({ sys1, sys1again, sys2, arrRes, strRes, noRes, blocked, hookKeys: Object.keys(hooks).sort(), types: subscribeOpts.types, aborted: subscribeOpts.signal.aborted, id: def.id }));
process.exit(0);
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
		Sys1    struct{ System []map[string]string }  `json:"sys1"`
		Again   struct{ System []map[string]string }  `json:"sys1again"`
		Sys2    struct{ System []map[string]string }  `json:"sys2"`
		ArrRes  struct{ Content []map[string]string } `json:"arrRes"`
		StrRes  struct{ Content string }              `json:"strRes"`
		NoRes   struct{ Content []map[string]string } `json:"noRes"`
		Blocked string                                `json:"blocked"`
		Keys    []string                              `json:"hookKeys"`
		Types   []string                              `json:"types"`
		Aborted bool                                  `json:"aborted"`
		ID      string                                `json:"id"`
	}
	if err := json.Unmarshal(out[strings.Index(string(out), "{"):], &got); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}

	if got.ID != "nerv.bridge" {
		t.Errorf("plugin id = %q", got.ID)
	}
	wantKeys := "session.context,tool.execute.after,tool.execute.before"
	if strings.Join(got.Keys, ",") != wantKeys {
		t.Errorf("hooks = %v, want %s", got.Keys, wantKeys)
	}
	// Session context lands in the system prompt on every request,
	// pushed as a v2 text part, per session.
	if len(got.Sys1.System) != 2 || got.Sys1.System[1]["text"] != "SESSION-CTX" || got.Sys1.System[1]["type"] != "text" {
		t.Errorf("session context not applied: %v", got.Sys1.System)
	}
	if len(got.Again.System) != 1 || got.Again.System[0]["text"] != "SESSION-CTX" {
		t.Errorf("context not re-applied on later request: %v", got.Again.System)
	}
	if len(got.Sys2.System) != 2 || got.Sys2.System[1]["text"] != "SESSION-CTX" {
		t.Errorf("session.created path lost context: %v", got.Sys2.System)
	}
	// Tool context appended to every shape of v2 result content.
	if n := len(got.ArrRes.Content); n != 2 || got.ArrRes.Content[1]["text"] != "TOOL-CTX" {
		t.Errorf("array content: %v", got.ArrRes.Content)
	}
	if got.StrRes.Content != "plain\nTOOL-CTX" {
		t.Errorf("string content = %q", got.StrRes.Content)
	}
	if len(got.NoRes.Content) != 1 || got.NoRes.Content[0]["text"] != "TOOL-CTX" {
		t.Errorf("absent content: %v", got.NoRes.Content)
	}
	if got.Blocked != "nerv: rm denied" {
		t.Errorf("execute.before did not throw the block message: %q", got.Blocked)
	}
	if !got.Aborted {
		t.Error("cleanup did not abort the event subscription")
	}
	wantTypes := "session.created,session.deleted,session.execution.succeeded,session.execution.interrupted," +
		"session.execution.failed,session.compaction.ended,permission.asked,permission.replied,tui.prompt.append"
	if strings.Join(got.Types, ",") != wantTypes {
		t.Errorf("subscribed types = %v", got.Types)
	}

	argv, _ := os.ReadFile(filepath.Join(dir, "argv.log"))
	for _, l := range strings.Split(strings.TrimSpace(string(argv)), "\n") {
		if l != "--adapter opencode" {
			t.Errorf("nervd argv = %q, want --adapter opencode", l)
		}
	}

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
	// Skipped on purpose: the second ses_1 context request and the
	// ses_2 context request after session.created (session start fires
	// once per session), the repeated completed todo, permission reply
	// "once", and interrupted-by-shutdown.
	wantSeq := []string{
		"session.created",     // ses_1, first model request
		"session.created",     // ses_2, bus event
		"tool.execute.before", // shell
		"tool.execute.after",  // shell
		"tool.execute.after",  // read
		"tool.execute.after",  // grep
		"tool.execute.after",  // write
		"file.edited",
		"tool.execute.after", // patch
		"file.edited",
		"tool.execute.after", // todowrite #1
		"todo.updated",
		"tool.execute.after", // todowrite #2 (no new completed)
		"tool.execute.after", // failed shell
		"session.idle",
		"session.idle",
		"session.error",
		"session.compacted",
		"permission.asked",
		"permission.replied",
		"tui.prompt.append",
		"session.deleted",
		"tool.execute.before", // blocked rm
	}
	if strings.Join(types, ",") != strings.Join(wantSeq, ",") {
		t.Fatalf("event sequence\n got %v\nwant %v", types, wantSeq)
	}

	nth := func(typ string, n int) map[string]any {
		for _, m := range sent {
			if m["type"] == typ {
				if n == 0 {
					return m
				}
				n--
			}
		}
		return nil
	}
	if m := nth("tool.execute.before", 0); m["tool"] != "bash" || m["session_id"] != "ses_1" ||
		m["input"].(map[string]any)["command"] != "ls" {
		t.Errorf("before wire = %v", m)
	}
	if m := nth("tool.execute.after", 0); m["tool"] != "bash" || m["output"] != "ls out" {
		t.Errorf("after wire (shell->bash, joined text) = %v", m)
	}
	if m := nth("tool.execute.after", 1); m["tool"] != "read" ||
		m["input"].(map[string]any)["filePath"] != "/a/CLAUDE.md" {
		t.Errorf("after wire (path->filePath) = %v", m)
	}
	if m := nth("tool.execute.after", 7); m["error"] != "exit 1" || m["output"] != nil {
		t.Errorf("failed after wire = %v", m)
	}
	if m := nth("file.edited", 0); m["path"] != "/tmp/a.go" || m["session_id"] != "ses_1" {
		t.Errorf("file.edited (write) = %v", m)
	}
	if m := nth("file.edited", 1); m["path"] != "/proj/b.go" {
		t.Errorf("file.edited (patch) = %v", m)
	}
	if m := nth("todo.updated", 0); m["session_id"] != "ses_1" {
		t.Errorf("todo.updated = %v", m)
	} else if in := m["input"].([]any); len(in) != 1 || in[0].(map[string]any)["content"] != "a" {
		t.Errorf("todo.updated input = %v", in)
	}
	if m := nth("session.error", 0); m["error"] != "boom" {
		t.Errorf("session.error = %v", m)
	}
	if m := nth("permission.asked", 0); m["session_id"] != "ses_1" {
		t.Errorf("permission.asked = %v", m)
	} else if in := m["input"].(map[string]any); in["permission"] != "shell" || in["id"] != "per_1" {
		t.Errorf("permission.asked input = %v", in)
	}
	if m := nth("permission.replied", 0); m["decision"] != "deny" {
		t.Errorf("permission.replied = %v", m)
	}
	if m := nth("tui.prompt.append", 0); m["input"] != "hello" {
		t.Errorf("tui.prompt.append = %v", m)
	}
	if m := nth("session.deleted", 0); m["session_id"] != "ses_2" {
		t.Errorf("session.deleted = %v", m)
	}
}

// Rewrite on execute.before replaces event.input in place, mapping nerv's
// filePath back to the host's "path". Covers: shell command rewrite,
// reverse mapping, pass-through of unmapped keys, filePath/path
// precedence, hosts without the alias, non-object rewrite, and no-op on
// other events.
func TestBridgePluginV2_Rewrite(t *testing.T) {
	stubCase := `  *'"command":"ls"'*) echo '{"action":"rewrite","message":"w","rewrite":{"command":"echo rewritten","timeout":9}}';;
  *'"content":"c1"'*) echo '{"action":"rewrite","rewrite":{"filePath":"/new","content":"c1","mode":"x"}}';;
  *'"content":"c2"'*) echo '{"action":"rewrite","rewrite":{"filePath":"/fp","path":"/p","content":"c2"}}';;
  *'"content":"c3"'*) echo '{"action":"rewrite","rewrite":{"filePath":"/old3","path":"/p3","content":"c3"}}';;
  *'"content":"c4"'*) echo '{"action":"rewrite","rewrite":{"path":"/only-path","content":"c4"}}';;
  *'"filePath":"/native"'*) echo '{"action":"rewrite","rewrite":{"filePath":"/native2"}}';;
  *'"command":"bad"'*) echo '{"action":"rewrite","rewrite":[1]}';;
  *tool.execute.after*) echo '{"action":"rewrite","rewrite":{"x":1}}';;`
	driver := `import plugin from "./plugin.ts";
const hooks: Record<string, Function> = {};
const ctx: any = {
  tool: { hook: async (n: string, cb: Function) => { hooks["tool." + n] = cb; return { dispose: async () => {} }; } },
  session: { hook: async (n: string, cb: Function) => { hooks["session." + n] = cb; return { dispose: async () => {} }; } },
  event: { subscribe: () => ({ [Symbol.asyncIterator]: () => ({ next: () => new Promise(() => {}) }) }) },
};
const def: any = plugin;
const cleanup: any = await def.setup(ctx);
const warns: string[] = [];
console.warn = (...a: any[]) => { warns.push(a.join(" ")); };

const before = async (tool: string, input: any) => {
  const ev: any = { tool, sessionID: "s", id: "c", input };
  const ref = ev.input;
  await hooks["tool.execute.before"](ev);
  return { input: ev.input, same: ref === ev.input };
};
const shell = await before("shell", { command: "ls", description: "d", workdir: "/w" });
const mapped = await before("write", { path: "/old", content: "c1" });
const both = await before("write", { path: "/old", content: "c2" });
const onlyPathChanged = await before("write", { path: "/old3", content: "c3" });
const pathOnly = await before("write", { path: "/old", content: "c4" });
const native = await before("read", { filePath: "/native" });
const bad = await before("shell", { command: "bad" });

const after: any = { tool: "shell", sessionID: "s", id: "c", input: { command: "zzz" }, status: "completed", result: { content: "o" } };
await hooks["tool.execute.after"](after);

console.log(JSON.stringify({ shell, mapped, both, onlyPathChanged, pathOnly, native, bad, after, warns }));
await cleanup();
process.exit(0);
`
	got := runBun(t, GeneratePluginV2, stubCase, driver)
	eq := func(name string, v any, want string) {
		t.Helper()
		b, _ := json.Marshal(v)
		if string(b) != want {
			t.Errorf("%s = %s, want %s", name, b, want)
		}
	}
	// Go marshals map keys sorted.
	eq("shell", got["shell"], `{"input":{"command":"echo rewritten","timeout":9},"same":true}`)
	eq("mapped filePath->path", got["mapped"], `{"input":{"content":"c1","mode":"x","path":"/new"},"same":true}`)
	eq("both: filePath wins", got["both"], `{"input":{"content":"c2","path":"/fp"},"same":true}`)
	eq("stale filePath, path changed: path wins", got["onlyPathChanged"], `{"input":{"content":"c3","path":"/p3"},"same":true}`)
	eq("path only passes through", got["pathOnly"], `{"input":{"content":"c4","path":"/only-path"},"same":true}`)
	eq("host without path alias", got["native"], `{"input":{"filePath":"/native2"},"same":true}`)
	eq("non-object rewrite ignored", got["bad"], `{"input":{"command":"bad"},"same":true}`)
	eq("after untouched", got["after"], `{"id":"c","input":{"command":"zzz"},"result":{"content":"o"},"sessionID":"s","status":"completed","tool":"shell"}`)
	warns, _ := json.Marshal(got["warns"])
	for _, w := range []string{"warning: w", "rewrite decision ignored (needs an object rewrite)", "only tool.execute.before supports rewrite"} {
		if !strings.Contains(string(warns), w) {
			t.Errorf("missing warning %q in %s", w, warns)
		}
	}
}
