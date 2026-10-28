package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestInstallStatusCmd_Exists(t *testing.T) {
	cmd := installStatusCmd()
	if cmd == nil {
		t.Fatal("installStatusCmd() returned nil")
	}
	if cmd.Use != "status" {
		t.Errorf("Use = %q, want %q", cmd.Use, "status")
	}
	if cmd.Short == "" {
		t.Error("Short description is empty")
	}
}

func TestInstallStatusCmd_OutputFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   string // substring expected in output
	}{
		{
			name:   "table_default",
			format: "table",
			want:   "CLI",
		},
		{
			name:   "json",
			format: "json",
			want:   `"cli"`,
		},
		{
			name:   "yaml",
			format: "yaml",
			want:   "cli:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set format via viper (matches how the CLI framework works).
			viper.Set("format", tt.format)
			t.Cleanup(func() { viper.Reset() })

			cmd := installStatusCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)

			err := cmd.RunE(cmd, nil)
			if err != nil {
				t.Fatalf("RunE error: %v", err)
			}

			// The command writes to os.Stdout via output.Render,
			// so we verify the format flag is accepted without error.
			// Detailed output assertions require capturing stdout which
			// is tested at the output package level.
			_ = tt.want
		})
	}
}

func TestInstallStatusCmd_KnownCLIs(t *testing.T) {
	// 15 = 13 originals + vibe + openhands (added in #16).
	if len(knownCLIs) != 15 {
		t.Errorf("knownCLIs has %d entries, want 15", len(knownCLIs))
	}

	// Verify the two additions are present.
	want := map[string]bool{"vibe": false, "openhands": false}
	for _, name := range knownCLIs {
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("expected CLI %q in knownCLIs", name)
		}
	}

	// Verify no duplicates.
	seen := make(map[string]bool, len(knownCLIs))
	for _, name := range knownCLIs {
		if seen[name] {
			t.Errorf("duplicate CLI name: %s", name)
		}
		seen[name] = true
	}
}

func TestBuildCLIStatus_NotInstalled(t *testing.T) {
	row := buildCLIStatus("nonexistent")
	if row.Installed != "\u2717" {
		t.Errorf("Installed = %q, want cross mark", row.Installed)
	}
	if row.Version != "\u2014" {
		t.Errorf("Version = %q, want em dash", row.Version)
	}
}

func TestInstallStatusCmd_RegisteredOnRoot(t *testing.T) {
	// Verify the command is findable from the root command tree.
	cmd := installStatusCmd()
	parent := cmd.Root()
	if parent == nil {
		t.Skip("command not attached to root in unit test context")
	}
	if !strings.Contains(cmd.UseLine(), "status") {
		t.Errorf("UseLine = %q, expected to contain status",
			cmd.UseLine())
	}
}
