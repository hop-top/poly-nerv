package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"hop.top/kit/go/console/output"
	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/events"
)

// lintIssue represents a single lint finding.
type lintIssue struct {
	Severity string `table:"Severity" json:"severity" yaml:"severity"`
	File     string `table:"File"     json:"file"     yaml:"file"`
	Line     int    `table:"Line"     json:"line"     yaml:"line"`
	Hook     string `table:"Hook"     json:"hook"     yaml:"hook"`
	Message  string `table:"Message"  json:"message"  yaml:"message"`
}

func lintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lint [path]",
		Short: "Lint hooks.yaml for errors and warnings",
		Long: `Validate hooks.yaml against the nerv spec:
- YAML syntax and schema structure
- Event names are valid nerv events
- Handler binaries exist and are executable
- Handler responds to empty envelope on stdin`,
		Args: cobra.MaximumNArgs(1),
		RunE: runLint,
		Annotations: map[string]string{
			"kit/side-effect":    "read",
			"kit/idempotent":     "yes",
			"kit/top-level-verb": "true",
		},
	}
}

func runLint(_ *cobra.Command, args []string) error {
	paths := config.LayerPaths()
	if len(args) == 1 {
		paths = []string{args[0]}
	}

	var issues []lintIssue

	for _, p := range paths {
		fileIssues := lintFile(p)
		issues = append(issues, fileIssues...)
	}

	format := viper.GetString("format")
	if format == "" {
		format = output.Table
	}

	if len(issues) == 0 {
		if format == output.Table {
			_, _ = fmt.Fprintln(os.Stdout, "No issues found.")
		}
		return nil
	}

	if err := output.Render(os.Stdout, format, issues); err != nil {
		return err
	}

	// Exit 1 if any errors found.
	for _, iss := range issues {
		if iss.Severity == "error" {
			os.Exit(1)
		}
	}
	return nil
}

// lintFile validates a single hooks.yaml file.
func lintFile(path string) []lintIssue {
	var issues []lintIssue

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // missing layer is fine
		}
		issues = append(issues, lintIssue{
			Severity: "error",
			File:     path,
			Message:  fmt.Sprintf("cannot read: %v", err),
		})
		return issues
	}

	// Stage 1: YAML parse check.
	var raw yaml.Node
	if err := yaml.Unmarshal(data, &raw); err != nil {
		issues = append(issues, lintIssue{
			Severity: "error",
			File:     path,
			Line:     1,
			Message:  fmt.Sprintf("invalid YAML: %v", err),
		})
		return issues
	}

	// Stage 2: schema structure check.
	var cfg config.HooksConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		issues = append(issues, lintIssue{
			Severity: "error",
			File:     path,
			Message:  fmt.Sprintf("schema mismatch: %v", err),
		})
		return issues
	}

	if cfg.Version == "" {
		issues = append(issues, lintIssue{
			Severity: "warning",
			File:     path,
			Line:     1,
			Message:  "missing 'version' field",
		})
	}

	// Stage 3: per-hook validation.
	for i, h := range cfg.Hooks {
		hookLine := findHookLine(data, i)

		if h.Name == "" {
			issues = append(issues, lintIssue{
				Severity: "error",
				File:     path,
				Line:     hookLine,
				Hook:     fmt.Sprintf("hooks[%d]", i),
				Message:  "hook missing 'name' field",
			})
		}

		// Validate event names.
		evNames := h.ResolveOn()
		if len(evNames) == 0 {
			issues = append(issues, lintIssue{
				Severity: "error",
				File:     path,
				Line:     hookLine,
				Hook:     h.Name,
				Message:  "hook missing 'on' field",
			})
		}
		for _, ev := range evNames {
			if !events.IsValid(events.Event(ev)) {
				issues = append(issues, lintIssue{
					Severity: "error",
					File:     path,
					Line:     hookLine,
					Hook:     h.Name,
					Message:  fmt.Sprintf("unknown event %q", ev),
				})
			}
		}

		// Validate handler.
		if h.Run == "" {
			issues = append(issues, lintIssue{
				Severity: "error",
				File:     path,
				Line:     hookLine,
				Hook:     h.Name,
				Message:  "hook missing 'run' field",
			})
			continue
		}

		handlerIssues := lintHandler(path, hookLine, h)
		issues = append(issues, handlerIssues...)
	}

	return issues
}

// lintHandler checks that a handler binary exists and is executable.
func lintHandler(file string, line int, h config.HookEntry) []lintIssue {
	var issues []lintIssue

	parts := strings.Fields(h.Run)
	bin := parts[0]

	resolved, err := exec.LookPath(bin)
	if err != nil {
		// Check if it's an absolute/relative path.
		if filepath.IsAbs(bin) || strings.HasPrefix(bin, ".") {
			if _, statErr := os.Stat(bin); statErr != nil {
				issues = append(issues, lintIssue{
					Severity: "error",
					File:     file,
					Line:     line,
					Hook:     h.Name,
					Message:  fmt.Sprintf("handler %q not found", bin),
				})
				return issues
			}
			resolved = bin
		} else {
			issues = append(issues, lintIssue{
				Severity: "error",
				File:     file,
				Line:     line,
				Hook:     h.Name,
				Message:  fmt.Sprintf("handler %q not on PATH", bin),
			})
			return issues
		}
	}

	info, err := os.Stat(resolved)
	if err != nil {
		issues = append(issues, lintIssue{
			Severity: "error",
			File:     file,
			Line:     line,
			Hook:     h.Name,
			Message:  fmt.Sprintf("cannot stat handler %s: %v", resolved, err),
		})
		return issues
	}

	if info.Mode()&0o111 == 0 {
		issues = append(issues, lintIssue{
			Severity: "error",
			File:     file,
			Line:     line,
			Hook:     h.Name,
			Message:  fmt.Sprintf("handler %s is not executable", resolved),
		})
	}

	// Probe: send empty JSON envelope to stdin.
	cmd := exec.Command("sh", "-c", h.Run)
	cmd.Stdin = strings.NewReader("{}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Exit code 0-3 are all valid per handler-contract.
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			if code >= 4 {
				issues = append(issues, lintIssue{
					Severity: "warning",
					File:     file,
					Line:     line,
					Hook:     h.Name,
					Message: fmt.Sprintf(
						"handler exited %d on empty envelope (expected 0-3)",
						code,
					),
				})
			}
		} else {
			issues = append(issues, lintIssue{
				Severity: "warning",
				File:     file,
				Line:     line,
				Hook:     h.Name,
				Message:  fmt.Sprintf("handler probe failed: %v", err),
			})
		}
	}

	// If stdout is non-empty, try to parse as JSON.
	trimmed := strings.TrimSpace(string(out))
	if trimmed != "" {
		var parsed map[string]any
		if jsonErr := json.Unmarshal([]byte(trimmed), &parsed); jsonErr != nil {
			issues = append(issues, lintIssue{
				Severity: "warning",
				File:     file,
				Line:     line,
				Hook:     h.Name,
				Message:  "handler stdout is not valid JSON",
			})
		}
	}

	return issues
}

// findHookLine approximates the YAML line number for the i-th hook.
func findHookLine(data []byte, hookIdx int) int {
	lines := strings.Split(string(data), "\n")
	hookCount := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- name:") {
			if hookCount == hookIdx {
				return i + 1 // 1-based
			}
			hookCount++
		}
	}
	return 0
}
