//go:build !windows

package opencode

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func opencodeV1BinOrSkip(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("NERV_OPENCODE_V1_BIN")
	if bin == "" {
		t.Skip("NERV_OPENCODE_V1_BIN not set (path to an opencode 1.x binary)")
	}
	return bin
}

// runOpencodeV1 is runOpencodeV2With for OpenCode 1.x: same install,
// handlers and fake LLM, v1 provider config and CLI flags.
func runOpencodeV1(t *testing.T, ocBin string, llm *fakeLLM, handlers map[string]string) v2Run {
	t.Helper()
	cfg := fmt.Sprintf(`{"model":"fake/fake","permission":{"bash":"allow","edit":"allow","external_directory":"allow"},`+
		`"provider":{"fake":{"npm":"@ai-sdk/openai-compatible",`+
		`"options":{"baseURL":%q,"apiKey":"x"},"models":{"fake":{"name":"fake","tool_call":true}}}}}`,
		llm.srv.URL+"/v1")
	return runOpencodeReal(t, ocBin, cfg, []string{"run", "-m", "fake/fake", "do it"}, llm, handlers)
}

// A nerv rewrite on PreToolUse changes what the real opencode 1.x runs.
func TestRealOpencodeV1_PreToolUseRewriteShell(t *testing.T) {
	bin := opencodeV1BinOrSkip(t)
	marks := t.TempDir()
	asked, rewritten := filepath.Join(marks, "asked"), filepath.Join(marks, "rewritten")
	llm := newFakeLLMTool(t, "bash", map[string]any{"command": "touch " + asked, "description": "run"})
	r := runOpencodeV1(t, bin, llm, map[string]string{
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

// v1 names the file argument filePath, as nerv does: no mapping needed.
func TestRealOpencodeV1_PreToolUseRewritePath(t *testing.T) {
	bin := opencodeV1BinOrSkip(t)
	marks := t.TempDir()
	asked, rewritten := filepath.Join(marks, "asked.txt"), filepath.Join(marks, "rewritten.txt")
	llm := newFakeLLMTool(t, "write", map[string]any{"filePath": asked, "content": "hello"})
	r := runOpencodeV1(t, bin, llm, map[string]string{
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

// Malformed rewrite on the real 1.x host: the tool call must not run the
// original command. The log records how the host reports it.
func TestRealOpencodeV1_PreToolUseRewriteMalformed(t *testing.T) {
	bin := opencodeV1BinOrSkip(t)
	for name, rw := range map[string]string{
		"wrong-type":  `{"command":42}`,
		"missing-arg": `{"description":"only"}`,
	} {
		t.Run(name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "ran")
			llm := newFakeLLMTool(t, "bash", map[string]any{"command": "touch " + marker, "description": "run"})
			runOpencodeV1(t, bin, llm, map[string]string{
				"PreToolUse": "#!/bin/sh\ncat >/dev/null\n" +
					`echo '{"action":"rewrite","rewrite":` + rw + `}'` + "\nexit 3\n",
			})
			if _, err := os.Stat(marker); err == nil {
				t.Fatal("tool ran the original command under a malformed rewrite")
			}
		})
	}
}
