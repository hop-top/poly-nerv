//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/dispatch"
	"hop.top/nerv/internal/envelope"
)

// Marketplace handlers run through the same path as `nerv test`
// (config.Load -> Compile -> testSingleEvent) with canonical events and
// payloads (spec/events.yaml), and must return decisions per
// spec/handler-contract.md: exit code drives the action, warn/block
// carry a message.

// marketplaceDir resolves the repo's marketplace/ before a test chdirs.
func marketplaceDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "marketplace"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("marketplace dir: %v", err)
	}
	return dir
}

// marketplaceEnv isolates config lookup and switches into an empty
// project directory.
func marketplaceEnv(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not in PATH")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	chdirTemp(t)
}

func writeHooksYAML(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(".nerv", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".nerv", "hooks.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// installStarter copies the agents-md starter into .nerv/ exactly as
// its README instructs.
func installStarter(t *testing.T, mkt string) {
	t.Helper()
	src := filepath.Join(mkt, "starters", "agents-md")
	data, err := os.ReadFile(filepath.Join(src, "hooks.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	writeHooksYAML(t, string(data))
	if err := os.MkdirAll(filepath.Join(".nerv", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	scripts, err := filepath.Glob(filepath.Join(src, "hooks", "*.sh"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("starter scripts: %v (%d found)", err, len(scripts))
	}
	for _, s := range scripts {
		b, err := os.ReadFile(s)
		if err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(".nerv", "hooks", filepath.Base(s))
		if err := os.WriteFile(dst, b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// envelopeWith builds the `nerv test` synthetic envelope for event and
// replaces its payload with the given canonical payload.
func envelopeWith(t *testing.T, event string, payload map[string]any) *envelope.Envelope {
	t.Helper()
	env := buildSyntheticEnvelope(event, "", "")
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	env.Payload = b
	return env
}

func runEnvelope(t *testing.T, env *envelope.Envelope) []testResult {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return testSingleEvent(config.Compile(cfg), dispatch.NewMatcher(), dispatch.NewRunner(), env)
}

// decisionFor returns the result for hook, failing if it did not run.
func decisionFor(t *testing.T, results []testResult, hook string) testResult {
	t.Helper()
	for _, r := range results {
		if r.Hook == hook {
			return r
		}
	}
	t.Fatalf("hook %q did not run; results: %+v", hook, results)
	return testResult{}
}

func assertDecision(t *testing.T, r testResult, want string) {
	t.Helper()
	if r.Decision != want {
		t.Fatalf("%s: decision = %q, want %q (%+v)", r.Hook, r.Decision, want, r)
	}
	if (want == "warn" || want == "block") && strings.TrimSpace(r.Message) == "" {
		t.Fatalf("%s: %s decision carries no message (%+v)", r.Hook, want, r)
	}
}

// fakeAWSKeyID is assembled at runtime so no source line holds a
// credential-shaped literal that secret scanners would flag.
var fakeAWSKeyID = "AKIA" + "ABCDEFGHIJKLMNOP"

func bash(cmd string) map[string]any {
	return map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": cmd},
	}
}

func TestMarketplace_BlockRmRf(t *testing.T) {
	mkt := marketplaceDir(t)
	marketplaceEnv(t)
	writeHooksYAML(t, `version: "1.0-draft"
hooks:
  - name: block-rm-rf
    on: PreToolUse
    match: "Bash"
    run: `+filepath.Join(mkt, "examples", "block-rm-rf", "handler.sh")+`
`)

	block := runEnvelope(t, buildSyntheticEnvelope("PreToolUse", "Bash", "rm -rf /"))
	assertDecision(t, decisionFor(t, block, "block-rm-rf"), "block")

	allow := runEnvelope(t, buildSyntheticEnvelope("PreToolUse", "Bash", "ls -la"))
	assertDecision(t, decisionFor(t, allow, "block-rm-rf"), "allow")
}

func TestMarketplace_AuditLog(t *testing.T) {
	mkt := marketplaceDir(t)
	marketplaceEnv(t)
	logFile := filepath.Join(t.TempDir(), "audit.log")
	t.Setenv("NERV_AUDIT_LOG", logFile)
	writeHooksYAML(t, `version: "1.0-draft"
hooks:
  - name: audit-log
    on: PreToolUse
    run: `+filepath.Join(mkt, "examples", "audit-log", "handler.sh")+`
`)

	res := runEnvelope(t, buildSyntheticEnvelope("PreToolUse", "Bash", "ls"))
	assertDecision(t, decisionFor(t, res, "audit-log"), "allow")

	b, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("audit log not written: %v", err)
	}
	if !strings.Contains(string(b), "event=PreToolUse tool=Bash") {
		t.Fatalf("audit log line missing event/tool: %q", b)
	}
}

func TestMarketplace_SlowQueryWarn(t *testing.T) {
	mkt := marketplaceDir(t)
	marketplaceEnv(t)
	writeHooksYAML(t, `version: "1.0-draft"
hooks:
  - name: slow-query-warn
    on: PostToolUse
    match: "Bash"
    run: `+filepath.Join(mkt, "examples", "slow-query-warn", "handler.sh")+`
`)

	slow := bash("make build")
	slow["tool_response"] = map[string]any{"stdout": "ok"}
	slow["duration_ms"] = 45000
	res := runEnvelope(t, envelopeWith(t, "PostToolUse", slow))
	assertDecision(t, decisionFor(t, res, "slow-query-warn"), "warn")

	fast := bash("ls")
	fast["duration_ms"] = 1200
	res = runEnvelope(t, envelopeWith(t, "PostToolUse", fast))
	assertDecision(t, decisionFor(t, res, "slow-query-warn"), "allow")
}

func TestMarketplace_TemplateHandlers(t *testing.T) {
	mkt := marketplaceDir(t)
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not in PATH")
	}
	bin := filepath.Join(t.TempDir(), "handler")
	build := exec.Command(goBin, "build", "-o", bin, filepath.Join(mkt, "template", "handler.go"))
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build template handler.go: %v\n%s", err, out)
	}
	marketplaceEnv(t)
	writeHooksYAML(t, `version: "1.0-draft"
hooks:
  - name: template-sh
    on: PreToolUse
    run: `+filepath.Join(mkt, "template", "handler.sh")+`
  - name: template-go
    on: PreToolUse
    run: `+bin+`
`)

	res := runEnvelope(t, buildSyntheticEnvelope("PreToolUse", "Bash", "ls"))
	assertDecision(t, decisionFor(t, res, "template-sh"), "allow")
	assertDecision(t, decisionFor(t, res, "template-go"), "allow")
}

func TestMarketplace_StarterAgentsMD(t *testing.T) {
	mkt := marketplaceDir(t)
	if _, err := exec.LookPath("file"); err != nil {
		t.Skip("file(1) not in PATH")
	}
	marketplaceEnv(t)
	installStarter(t, mkt)

	write := func(path string) map[string]any {
		abs, err := filepath.Abs(path)
		if err != nil {
			t.Fatal(err)
		}
		return map[string]any{
			"tool_name":     "Write",
			"tool_input":    map[string]any{"file_path": abs},
			"tool_response": map[string]any{"success": true},
		}
	}

	t.Run("validate-agents-md", func(t *testing.T) {
		long := "# Agents\n\n## Rules\n\n" + strings.Repeat("x", 300) + "\n"
		if err := os.WriteFile("AGENTS.md", []byte(long), 0o644); err != nil {
			t.Fatal(err)
		}
		res := runEnvelope(t, envelopeWith(t, "PostToolUse", write("AGENTS.md")))
		assertDecision(t, decisionFor(t, res, "validate-agents-md"), "warn")

		// Same content in a non-agents file: out of scope, allowed.
		if err := os.WriteFile("notes.md", []byte(long), 0o644); err != nil {
			t.Fatal(err)
		}
		res = runEnvelope(t, envelopeWith(t, "PostToolUse", write("notes.md")))
		assertDecision(t, decisionFor(t, res, "validate-agents-md"), "allow")
	})

	t.Run("warn-large-writes", func(t *testing.T) {
		t.Setenv("NERV_LARGE_WRITE_KB", "1")
		if err := os.WriteFile("big.bin", make([]byte, 4096), 0o644); err != nil {
			t.Fatal(err)
		}
		res := runEnvelope(t, envelopeWith(t, "PostToolUse", write("big.bin")))
		assertDecision(t, decisionFor(t, res, "warn-large-writes"), "warn")

		if err := os.WriteFile("small.txt", []byte("hi"), 0o644); err != nil {
			t.Fatal(err)
		}
		res = runEnvelope(t, envelopeWith(t, "PostToolUse", write("small.txt")))
		assertDecision(t, decisionFor(t, res, "warn-large-writes"), "allow")
	})

	t.Run("enforce-conventional-commits", func(t *testing.T) {
		res := runEnvelope(t, buildSyntheticEnvelope("PreToolUse", "Bash", `git commit -m "updated stuff"`))
		assertDecision(t, decisionFor(t, res, "enforce-conventional-commits"), "warn")

		res = runEnvelope(t, buildSyntheticEnvelope("PreToolUse", "Bash", `git commit -m "feat(cli): add flag"`))
		assertDecision(t, decisionFor(t, res, "enforce-conventional-commits"), "allow")
	})

	t.Run("block-secrets", func(t *testing.T) {
		res := runEnvelope(t, buildSyntheticEnvelope("PreToolUse", "Bash", "export KEY="+fakeAWSKeyID))
		assertDecision(t, decisionFor(t, res, "block-secrets"), "block")

		out := bash("cat .env")
		out["tool_response"] = map[string]any{"stdout": "token=ghp_" + strings.Repeat("a", 36)}
		res = runEnvelope(t, envelopeWith(t, "PostToolUse", out))
		assertDecision(t, decisionFor(t, res, "block-secrets"), "block")

		res = runEnvelope(t, buildSyntheticEnvelope("PreToolUse", "Bash", "ls"))
		assertDecision(t, decisionFor(t, res, "block-secrets"), "allow")
	})
}
