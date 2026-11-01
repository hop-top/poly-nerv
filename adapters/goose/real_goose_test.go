//go:build !windows

package goose

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

// fakeLLM is an OpenAI-compatible chat endpoint. With toolCall, the first
// completion asks for a shell call; later ones answer "done". It records
// every request body's messages as JSON.
type fakeLLM struct {
	mu       sync.Mutex
	toolCall bool
	seen     []string
	srv      *httptest.Server
}

func newFakeLLM(t *testing.T, toolCall bool, shellCmd string) *fakeLLM {
	f := &fakeLLM{toolCall: toolCall}
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
		if f.toolCall && !hasTool {
			args, _ := json.Marshal(map[string]any{"command": shellCmd})
			_, _ = fmt.Fprint(w, chunk(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{
				"index": 0, "id": "call_1", "type": "function",
				"function": map[string]any{"name": "shell", "arguments": string(args)}}}}, nil))
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

// runGoose installs the nerv hooks the way `nerv install` renders them
// (Adapter.MergeConfig) into an isolated HOME, points nervd at a handler
// script, and runs the real goose CLI against the fake LLM.
func runGoose(t *testing.T, gooseBin string, llm *fakeLLM, handler string, nervEvents ...string) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	work := filepath.Join(root, "work")
	for _, d := range []string{home, bin, work} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	nervd := filepath.Join(bin, "nervd")
	build := exec.Command("go", "build", "-o", nervd, "../../cmd/nervd")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build nervd: %v\n%s", err, out)
	}

	merged, err := (&Adapter{}).MergeConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	hooksPath := filepath.Join(home, ".agents", "plugins", "nerv", "hooks", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hooksPath, merged, 0o600); err != nil {
		t.Fatal(err)
	}

	script := filepath.Join(root, "handler.sh")
	if err := os.WriteFile(script, []byte(handler), 0o700); err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Join(home, ".config", "nerv")
	if err := os.MkdirAll(cfgDir, 0o750); err != nil {
		t.Fatal(err)
	}
	hooksYAML := "version: \"1\"\nhooks:\n"
	for _, ev := range nervEvents {
		hooksYAML += fmt.Sprintf("  - name: gate-%s\n    on: %s\n    run: %s\n", strings.ToLower(ev), ev, script)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "hooks.yaml"), []byte(hooksYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(gooseBin, "run", "-t", "do it", "--no-session")
	cmd.Dir = work
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + bin + ":/usr/bin:/bin",
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"GOOSE_PROVIDER=openai",
		"GOOSE_MODEL=fake",
		"OPENAI_API_KEY=x",
		"OPENAI_HOST=" + llm.srv.URL,
		"GOOSE_DISABLE_KEYRING=1",
	}
	done := make(chan error, 1)
	var out []byte
	go func() {
		var e error
		out, e = cmd.CombinedOutput()
		done <- e
	}()
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("goose timed out")
	}
	t.Logf("goose output:\n%s", out)
}

func gooseBinOrSkip(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("NERV_GOOSE_BIN")
	if bin == "" {
		t.Skip("NERV_GOOSE_BIN not set (path to a goose >= 1.55.0 binary)")
	}
	return bin
}

// A nerv block on PreToolUse, delivered through `nervd --adapter goose`,
// stops the real goose from running the tool.
func TestRealGoose_PreToolUseBlock(t *testing.T) {
	bin := gooseBinOrSkip(t)
	marker := filepath.Join(t.TempDir(), "ran")
	llm := newFakeLLM(t, true, "touch "+marker)
	handler := "#!/bin/sh\ncat >/dev/null\n" +
		`echo '{"action":"block","message":"nerv says no"}'` + "\nexit 2\n"
	runGoose(t, bin, llm, handler, "PreToolUse")
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("tool ran despite nerv block")
	}
	if tr := llm.transcript(); !strings.Contains(tr, "Tool call denied by policy hook") ||
		!strings.Contains(tr, "nerv says no") {
		t.Fatalf("model never saw the denial:\n%s", tr)
	}
}

// Control: an allow lets the tool run (proves the block test is not
// passing because the tool could never run).
func TestRealGoose_PreToolUseAllow(t *testing.T) {
	bin := gooseBinOrSkip(t)
	marker := filepath.Join(t.TempDir(), "ran")
	llm := newFakeLLM(t, true, "touch "+marker)
	runGoose(t, bin, llm, "#!/bin/sh\ncat >/dev/null\nexit 0\n", "PreToolUse")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("tool did not run under allow: %v", err)
	}
}

// A nerv block on Stop keeps the turn going; the reason reaches the model.
func TestRealGoose_StopBlock(t *testing.T) {
	bin := gooseBinOrSkip(t)
	llm := newFakeLLM(t, false, "")
	state := filepath.Join(t.TempDir(), "blocked-once")
	handler := "#!/bin/sh\ncat >/dev/null\nif [ ! -e " + state + " ]; then touch " + state + "\n" +
		`echo '{"action":"block","message":"run the tests first"}'` + "\nexit 2\nfi\nexit 0\n"
	runGoose(t, bin, llm, handler, "Stop")
	if tr := llm.transcript(); !strings.Contains(tr, "blocked ending this turn") ||
		!strings.Contains(tr, "run the tests first") {
		t.Fatalf("model never saw the Stop block reason:\n%s", tr)
	}
}
