package aider

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	internal "hop.top/nerv/adapters/internal"
)

const cliName internal.CLIName = "aider"

// Adapter implements HookAdapter for Aider CLI.
type Adapter struct{}

// CLI returns the canonical adapter name.
func (a *Adapter) CLI() internal.CLIName { return cliName }

// Detect probes whether Aider is installed.
func (a *Adapter) Detect() (internal.DetectResult, error) {
	path, err := exec.LookPath("aider")
	if err != nil {
		// Check config dirs as fallback signal.
		for _, cp := range configPaths() {
			if _, err := os.Stat(cp); err == nil {
				return internal.DetectResult{
					Found: true,
					Info:  internal.CLIInfo{Name: cliName},
				}, nil
			}
		}
		return internal.DetectResult{Found: false}, nil
	}

	out, err := exec.Command(path, "--version").Output()
	version := ""
	if err == nil {
		version = strings.TrimSpace(string(out))
	}

	return internal.DetectResult{
		Found: true,
		Info: internal.CLIInfo{
			Name:    cliName,
			Version: version,
			Path:    path,
		},
	}, nil
}

// Capabilities reports which hook points Aider supports.
// Aider has no native hook system. The config nerv installs wires
// lint-cmd/test-cmd, which fire after edits are applied (FileChanged,
// PostToolUse); nothing fires before an edit or at process start/exit,
// so PreToolUse and session hooks are not claimed.
func (a *Adapter) Capabilities() internal.CapabilityMap {
	return internal.CapabilityMap{
		PreToolUse:  false,
		PostToolUse: true,
		PreSession:  false,
		PostSession: false,
		Streaming:   false,
	}
}

// ConfigPaths returns paths to Aider config files.
func (a *Adapter) ConfigPaths() []string {
	return configPaths()
}

// MergeConfig merges nerv hook entries into an existing Aider config.
// Aider uses YAML config (.aider.conf.yml); the edit is line-based so
// comments and unrelated keys survive byte for byte. It is idempotent
// and upgrades legacy `nerv dispatch aider ...` entries in place. A
// lint-cmd given as a YAML list is rejected rather than guessed at.
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	if len(existing) == 0 {
		return []byte(nervConfigBlock()), nil
	}
	out, err := wrapExistingKeys(string(existing))
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// Translate converts an Aider-native event to a nerv envelope.
// Since Aider has no native hook events, this handles the
// synthesized event format from the nerv bridge script.
func (a *Adapter) Translate(eventBytes []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(eventBytes, &m); err != nil {
		return nil, fmt.Errorf("aider: parse event: %w", err)
	}
	env, err := ToEnvelope(m)
	if err != nil {
		return nil, err
	}
	return json.Marshal(env)
}

// --- internal helpers ---

func configPaths() []string {
	paths := []string{".aider.conf.yml"}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append([]string{
			filepath.Join(home, ".aider.conf.yml"),
		}, paths...)
	}
	return paths
}

// Hook commands. Aider runs them through the shell and appends the
// edited file, shell-quoted, to lint-cmd only (aider/linter.py
// run_cmd); test-cmd gets no arguments (aider/commands.py cmd_test).
const (
	lintBridge = "nervd --adapter aider lint"
	testBridge = "nervd --adapter aider test"

	// legacy commands named a subcommand nerv never had.
	legacyLint = "nerv dispatch aider lint"
	legacyTest = "nerv dispatch aider test"
)

// chain wraps a user command so nerv gates it and
// both see the appended file: Aider's trailing word lands on the shell
// function call, which forwards "$@" to each. nerv runs first; block
// (exit 1) skips the original and its message reaches the model, any
// other result falls through to the original, whose output and exit
// status are what Aider sees.
func chain(bridge, orig string) string {
	return "f(){ " + bridge + ` "$@" && ` + orig + ` "$@"; }; f`
}

// nervConfigBlock returns the YAML config lines to add to .aider.conf.yml.
func nervConfigBlock() string {
	return "# nerv hook integration\n" +
		"lint-cmd: " + lintBridge + "\n" +
		"test-cmd: " + testBridge + "\n" +
		"auto-lint: true\n" +
		"auto-test: true\n"
}

// langPrefix matches Aider's per-language form "python: cmd"
// (aider/main.py parse_lint_cmds: ^[a-z]+:.*).
var langPrefix = regexp.MustCompile(`^([a-z]+):(.*)$`)

// splitLang separates an optional "lang:" prefix from the command.
func splitLang(v string) (lang, cmd string) {
	if m := langPrefix.FindStringSubmatch(v); m != nil {
		return m[1] + ": ", strings.TrimSpace(m[2])
	}
	return "", v
}

// wrapCmd returns the nerv-gated form of value v (already YAML-decoded)
// for a key whose fresh bridge is bridge, and whether it differs from v.
func wrapCmd(v, bridge, legacy string) string {
	lang, cmd := splitLang(v)
	switch {
	case cmd == "":
		return v
	case cmd == bridge || strings.HasPrefix(cmd, "f(){ "+bridge+" "):
		return v // current
	case cmd == legacy:
		return lang + bridge
	case strings.HasPrefix(cmd, legacy+" && "):
		cmd = strings.TrimSpace(strings.TrimPrefix(cmd, legacy+" && "))
		// A per-language original was glued after the legacy prefix.
		lang2, cmd2 := splitLang(cmd)
		if lang2 != "" {
			lang, cmd = lang2, cmd2
		} else {
			cmd = cmd2
		}
	}
	cmd = strings.TrimRight(strings.TrimSpace(cmd), "; \t")
	if cmd == "" {
		return lang + bridge
	}
	return lang + chain(bridge, cmd)
}

// yamlScalar decodes the inline value of "key: value". It supports
// plain, 'single' and "double" quoted scalars with trailing comments
// and reports ok=false for anything else (lists, block scalars, empty).
func yamlScalar(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "" || raw[0] == '[' || raw[0] == '{' || raw[0] == '|' ||
		raw[0] == '>' || raw[0] == '&' || raw[0] == '*' || raw[0] == '!':
		return "", false
	case raw[0] == '\'':
		var sb strings.Builder
		for i := 1; i < len(raw); i++ {
			if raw[i] != '\'' {
				sb.WriteByte(raw[i])
				continue
			}
			if i+1 < len(raw) && raw[i+1] == '\'' {
				sb.WriteByte('\'')
				i++
				continue
			}
			rest := strings.TrimSpace(raw[i+1:])
			if rest != "" && rest[0] != '#' {
				return "", false
			}
			return sb.String(), true
		}
		return "", false
	case raw[0] == '"':
		for i := 1; i < len(raw); i++ {
			if raw[i] == '\\' {
				i++
				continue
			}
			if raw[i] == '"' {
				rest := strings.TrimSpace(raw[i+1:])
				if rest != "" && rest[0] != '#' {
					return "", false
				}
				v, err := strconv.Unquote(raw[:i+1])
				return v, err == nil
			}
		}
		return "", false
	}
	if i := strings.Index(raw, " #"); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(raw), true
}

// yamlSingle renders v as a single-quoted YAML scalar.
func yamlSingle(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

// render writes key: value, quoting only when the value is not a safe
// plain scalar. Wrapped chains always quote: they contain "{", '"'
// and ';' which plain scalars tolerate, but an original with ": " or
// " #" would not survive.
func render(key, val string) string {
	if val == lintBridge || val == testBridge {
		return key + ": " + val
	}
	return key + ": " + yamlSingle(val)
}

// wrapExistingKeys rewrites lint-cmd/test-cmd to the nerv-gated form,
// preserving the user's original command, and ensures auto-lint and
// auto-test are on. Missing keys are appended. A lint-cmd/test-cmd
// whose value is not a scalar (a YAML list) is an error.
func wrapExistingKeys(content string) (string, error) {
	lines := strings.Split(content, "\n")
	var foundLint, foundTest, foundAutoLint, foundAutoTest bool

	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "lint-cmd:"):
			foundLint = true
			nl, err := rewriteLine(line, "lint-cmd", lintBridge, legacyLint)
			if err != nil {
				return "", err
			}
			lines[i] = nl
		case strings.HasPrefix(line, "test-cmd:"):
			foundTest = true
			nl, err := rewriteLine(line, "test-cmd", testBridge, legacyTest)
			if err != nil {
				return "", err
			}
			lines[i] = nl
		case strings.HasPrefix(line, "auto-lint:"):
			lines[i] = "auto-lint: true"
			foundAutoLint = true
		case strings.HasPrefix(line, "auto-test:"):
			lines[i] = "auto-test: true"
			foundAutoTest = true
		}
	}

	result := strings.Join(lines, "\n")
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	// Append any missing keys.
	var extra []string
	if !foundLint {
		extra = append(extra, "lint-cmd: "+lintBridge)
	}
	if !foundTest {
		extra = append(extra, "test-cmd: "+testBridge)
	}
	if !foundAutoLint {
		extra = append(extra, "auto-lint: true")
	}
	if !foundAutoTest {
		extra = append(extra, "auto-test: true")
	}
	if len(extra) > 0 {
		result += "\n# nerv hook integration\n"
		result += strings.Join(extra, "\n") + "\n"
	}
	return result, nil
}

func rewriteLine(line, key, bridge, legacy string) (string, error) {
	val, ok := yamlScalar(strings.TrimPrefix(line, key+":"))
	if !ok {
		return "", fmt.Errorf("aider: %s must be a single-line scalar to be "+
			"wrapped (YAML lists unsupported); add the nerv command by hand: %s",
			key, bridge)
	}
	nv := wrapCmd(val, bridge, legacy)
	if nv == val {
		return line, nil // current: byte-for-byte untouched
	}
	return render(key, nv), nil
}
