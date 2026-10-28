package aider

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
// Aider has no native hook system; all events are synthesized.
func (a *Adapter) Capabilities() internal.CapabilityMap {
	return internal.CapabilityMap{
		PreToolUse:  true,
		PostToolUse: true,
		PreSession:  true,
		PostSession: true,
		Streaming:   false,
	}
}

// ConfigPaths returns paths to Aider config files.
func (a *Adapter) ConfigPaths() []string {
	return configPaths()
}

// MergeConfig merges nerv hook entries into existing Aider config.
// Aider uses YAML config (.aider.conf.yml). We output YAML via
// string manipulation — no yaml library.
func (a *Adapter) MergeConfig(existing []byte) ([]byte, error) {
	content := string(existing)

	// Check if nerv hook is already configured.
	if strings.Contains(content, "nerv dispatch aider") {
		return existing, nil
	}

	if len(existing) == 0 {
		return []byte(nervConfigBlock()), nil
	}

	// Wrap existing lint-cmd/test-cmd with nerv dispatch, or append
	// new keys if they don't exist yet.
	content = wrapExistingKeys(content)

	return []byte(content), nil
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

// nervConfigBlock returns the YAML config lines to add to .aider.conf.yml.
func nervConfigBlock() string {
	return `# nerv hook integration
lint-cmd: nerv dispatch aider lint
test-cmd: nerv dispatch aider test
auto-lint: true
auto-test: true
`
}

// wrapExistingKeys replaces existing lint-cmd/test-cmd values with nerv
// wrappers that chain the original command, and ensures auto-lint/
// auto-test are enabled. Keys that don't exist are appended.
func wrapExistingKeys(content string) string {
	lines := strings.Split(content, "\n")
	foundLint := false
	foundTest := false
	foundAutoLint := false
	foundAutoTest := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "lint-cmd:") {
			existing := strings.TrimSpace(
				strings.TrimPrefix(trimmed, "lint-cmd:"),
			)
			lines[i] = "lint-cmd: nerv dispatch aider lint && " +
				existing
			foundLint = true
		} else if strings.HasPrefix(trimmed, "test-cmd:") {
			existing := strings.TrimSpace(
				strings.TrimPrefix(trimmed, "test-cmd:"),
			)
			lines[i] = "test-cmd: nerv dispatch aider test && " +
				existing
			foundTest = true
		} else if strings.HasPrefix(trimmed, "auto-lint:") {
			lines[i] = "auto-lint: true"
			foundAutoLint = true
		} else if strings.HasPrefix(trimmed, "auto-test:") {
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
		extra = append(extra, "lint-cmd: nerv dispatch aider lint")
	}
	if !foundTest {
		extra = append(extra, "test-cmd: nerv dispatch aider test")
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

	return result
}
