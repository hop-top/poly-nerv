//go:build !windows

package opencode

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeLLM is an OpenAI-compatible chat endpoint. With a tool set, the
// first completion that sees no tool result asks for a shell call; later
// ones answer "done". It records every request's messages as JSON.
type fakeLLM struct {
	mu sync.Mutex
	// tool/toolArgs, when tool is set, replace the shell call: the first
	// completion asks for that tool with those arguments.
	tool     string
	toolArgs map[string]any
	seen     []string
	srv      *httptest.Server
}

func newFakeLLM(t *testing.T, shellCmd string) *fakeLLM {
	return newFakeLLMTool(t, "shell", map[string]any{"command": shellCmd, "description": "run"})
}

// newFakeLLMTool asks the host for one call to the named tool.
func newFakeLLMTool(t *testing.T, tool string, args map[string]any) *fakeLLM {
	f := &fakeLLM{tool: tool, toolArgs: args}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"fake","object":"model"}]}`))
			return
		}
		var body struct {
			Messages []map[string]any `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body.Messages)
		f.mu.Lock()
		f.seen = append(f.seen, string(raw))
		f.mu.Unlock()
		hasTool := false
		for _, m := range body.Messages {
			if m["role"] == "tool" {
				hasTool = true
			}
		}
		chunk := func(delta map[string]any, fin any) string {
			b, _ := json.Marshal(map[string]any{
				"id": "c", "object": "chat.completion.chunk", "created": 1, "model": "fake",
				"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": fin}},
			})
			return "data: " + string(b) + "\n\n"
		}
		w.Header().Set("content-type", "text/event-stream")
		if f.tool != "" && !hasTool {
			args, _ := json.Marshal(f.toolArgs)
			_, _ = fmt.Fprint(w, chunk(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
				"index": 0, "id": "call_1", "type": "function",
				"function": map[string]any{"name": f.tool, "arguments": string(args)},
			}}}, nil))
			_, _ = fmt.Fprint(w, chunk(map[string]any{}, "tool_calls"))
		} else {
			_, _ = fmt.Fprint(w, chunk(map[string]any{"role": "assistant", "content": "done"}, nil))
			_, _ = fmt.Fprint(w, chunk(map[string]any{}, "stop"))
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLLM) transcript() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.seen, "\n")
}

func opencodeV2BinOrSkip(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("NERV_OPENCODE_V2_BIN")
	if bin == "" {
		t.Skip("NERV_OPENCODE_V2_BIN not set (path to an opencode 2.x binary)")
	}
	return bin
}

type v2Run struct {
	project string
	llm     *fakeLLM
	out     string
}

// runOpencodeV2 installs nerv into an isolated project with the real
// `nerv install add opencode`, wires nervd handler scripts through
// hooks.yaml, and runs the real opencode 2.x against the fake LLM.
// handlers maps a nerv event to the handler script body.
func runOpencodeV2(t *testing.T, ocBin string, shellCmd string, handlers map[string]string) v2Run {
	t.Helper()
	return runOpencodeV2With(t, ocBin, newFakeLLM(t, shellCmd), handlers)
}

// runOpencodeV2With is runOpencodeV2 against a caller-built fake LLM.
func runOpencodeV2With(t *testing.T, ocBin string, llm *fakeLLM, handlers map[string]string) v2Run {
	t.Helper()
	cfg := fmt.Sprintf(`{"model":"fake/fake","providers":{"fake":{`+
		`"package":"@opencode/ai/providers/openai-compatible",`+
		`"settings":{"baseURL":%q},"models":{"fake":{}}}}}`, llm.srv.URL+"/v1")
	return runOpencodeReal(t, ocBin, cfg, []string{"run", "--standalone", "--auto", "-m", "fake/fake", "do it"}, llm, handlers)
}

// runOpencodeReal installs nerv into an isolated project, wires the
// handlers, writes cfg as opencode.json and runs `ocBin args...`.
func runOpencodeReal(t *testing.T, ocBin, cfg string, args []string, llm *fakeLLM, handlers map[string]string) v2Run {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	project := filepath.Join(root, "proj")
	for _, d := range []string{home, bin, project} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ out, pkg string }{
		{"nervd", "../../cmd/nervd"}, {"nerv", "../../cmd/nerv"},
	} {
		build := exec.Command("go", "build", "-o", filepath.Join(bin, c.out), c.pkg)
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", c.out, err, out)
		}
	}
	if err := os.Symlink(ocBin, filepath.Join(bin, "opencode")); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(project, "opencode.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	env := []string{
		"HOME=" + home,
		"PATH=" + bin + ":/usr/bin:/bin",
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
	}

	install := exec.Command(filepath.Join(bin, "nerv"), "install", "add", "opencode")
	install.Dir, install.Env = project, env
	if out, err := install.CombinedOutput(); err != nil {
		t.Fatalf("nerv install add opencode: %v\n%s", err, out)
	}

	cfgDir := filepath.Join(home, ".config", "nerv")
	if err := os.MkdirAll(cfgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	hooksYAML := "version: \"1\"\nhooks:\n"
	for ev, body := range handlers {
		script := filepath.Join(root, "handler-"+ev+".sh")
		if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
		hooksYAML += fmt.Sprintf("  - name: gate-%s\n    on: %s\n    run: %s\n", strings.ToLower(ev), ev, script)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "hooks.yaml"), []byte(hooksYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(ocBin, args...)
	cmd.Dir, cmd.Env = project, env
	done := make(chan struct{})
	var out []byte
	go func() {
		out, _ = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("opencode timed out")
	}
	t.Logf("opencode output:\n%s", out)
	return v2Run{project: project, llm: llm, out: string(out)}
}

// waitFor polls for path: handler side effects land after the run exits
// only for fire-and-forget events.
func waitFor(path string) bool {
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// With nerv installed by `nerv install add opencode`, the real
// opencode 2.x: loads the v2 bridge, takes SessionStart context into the
// system prompt, appends PostToolUse context to the tool result, runs the
// allowed tool, and reports the session going idle (Stop).
func TestRealOpencodeV2_ContextAndLifecycle(t *testing.T) {
	bin := opencodeV2BinOrSkip(t)
	marks := t.TempDir()
	toolMarker := filepath.Join(marks, "ran")
	script := func(extra string) string { return "#!/bin/sh\ncat >/dev/null\n" + extra }
	r := runOpencodeV2(t, bin, "touch "+toolMarker, map[string]string{
		"SessionStart": script(`echo '{"action":"allow","context":"NERV-SESSION-CTX"}'` + "\nexit 0\n"),
		"PostToolUse":  script(`echo '{"action":"allow","context":"NERV-POST-CTX"}'` + "\nexit 0\n"),
		"Stop":         script("touch " + filepath.Join(marks, "stop") + "\nexit 0\n"),
	})
	if _, err := os.Stat(toolMarker); err != nil {
		t.Fatalf("tool did not run under allow: %v", err)
	}
	tr := r.llm.transcript()
	if !strings.Contains(tr, "NERV-SESSION-CTX") {
		t.Errorf("session context never reached the model:\n%.2000s", tr)
	}
	if !strings.Contains(tr, "NERV-POST-CTX") {
		t.Errorf("post-tool context never reached the model:\n%.2000s", tr)
	}
	if !waitFor(filepath.Join(marks, "stop")) {
		t.Error("Stop hook never fired (session.execution.succeeded -> session.idle)")
	}
}

// A nerv block on PreToolUse stops the real opencode 2.x from running
// the tool, and the block message reaches the model.
func TestRealOpencodeV2_PreToolUseBlock(t *testing.T) {
	bin := opencodeV2BinOrSkip(t)
	marker := filepath.Join(t.TempDir(), "ran")
	r := runOpencodeV2(t, bin, "touch "+marker, map[string]string{
		"PreToolUse": "#!/bin/sh\ncat >/dev/null\n" +
			`echo '{"action":"block","message":"nerv says no"}'` + "\nexit 2\n",
	})
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("tool ran despite nerv block")
	}
	if tr := r.llm.transcript(); !strings.Contains(tr, "nerv says no") {
		t.Fatalf("model never saw the block message:\n%.2000s", tr)
	}
}

// A nerv rewrite on PreToolUse changes what the real opencode 2.x runs:
// the shell tool executes the rewritten command, not the requested one.
func TestRealOpencodeV2_PreToolUseRewriteShell(t *testing.T) {
	bin := opencodeV2BinOrSkip(t)
	marks := t.TempDir()
	asked, rewritten := filepath.Join(marks, "asked"), filepath.Join(marks, "rewritten")
	r := runOpencodeV2(t, bin, "touch "+asked, map[string]string{
		"PreToolUse": "#!/bin/sh\ncat >/dev/null\n" +
			`echo '{"action":"rewrite","rewrite":{"command":"touch ` + rewritten + `","description":"run"}}'` + "\nexit 3\n",
	})
	if _, err := os.Stat(rewritten); err != nil {
		t.Fatalf("rewritten command did not run: %v\n%s", err, r.out)
	}
	if _, err := os.Stat(asked); err == nil {
		t.Fatal("original command ran despite rewrite")
	}
}

// The rewrite is in nerv's vocabulary (filePath); the bridge maps it back
// to the host's "path", so the real write tool lands the file elsewhere.
func TestRealOpencodeV2_PreToolUseRewritePath(t *testing.T) {
	bin := opencodeV2BinOrSkip(t)
	marks := t.TempDir()
	asked, rewritten := filepath.Join(marks, "asked.txt"), filepath.Join(marks, "rewritten.txt")
	llm := newFakeLLMTool(t, "write", map[string]any{"path": asked, "content": "hello"})
	r := runOpencodeV2With(t, bin, llm, map[string]string{
		"PreToolUse": "#!/bin/sh\ncat >/dev/null\n" +
			`echo '{"action":"rewrite","rewrite":{"filePath":"` + rewritten + `","content":"hello"}}'` + "\nexit 3\n",
	})
	if b, err := os.ReadFile(rewritten); err != nil || string(b) != "hello" {
		t.Fatalf("write did not land at rewritten path: %v %q\n%s", err, b, r.out)
	}
	if _, err := os.Stat(asked); err == nil {
		t.Fatal("original path was written despite rewrite")
	}
}

// Malformed rewrite (wrong type, missing required arg): records how the
// real host reacts. OpenCode re-validates the rewritten input, so the
// tool call errors cleanly instead of running.
func TestRealOpencodeV2_PreToolUseRewriteMalformed(t *testing.T) {
	bin := opencodeV2BinOrSkip(t)
	for name, rw := range map[string]string{
		"wrong-type":  `{"command":42}`,
		"missing-arg": `{"description":"only"}`,
	} {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			r := runOpencodeV2(t, bin, "touch "+marker, map[string]string{
				"PreToolUse": "#!/bin/sh\ncat >/dev/null\n" +
					`echo '{"action":"rewrite","rewrite":` + rw + `}'` + "\nexit 3\n",
			})
			if _, err := os.Stat(marker); err == nil {
				t.Fatal("tool ran the original command under a malformed rewrite")
			}
			t.Logf("model transcript: %.1500s", r.llm.transcript())
		})
	}
}
