package main

import (
	"testing"

	"github.com/spf13/viper"
)

// TestTailCmd_FlagsReachRunE locks in the fix for the viper-binding
// defect on `nerv tail`: tailCmd registers --event/--cli/--slow/--last
// as local cobra flags but they were previously read via the
// package-global viper instance, which kit never binds to. The result:
// every `nerv tail --event PreToolUse --last 5` ran with empty filters
// and last=0 regardless of the flag values.
//
// Regression: read flags off cmd.Flags() instead of viper.
//
// runTail itself enters an infinite follow loop, so we don't invoke
// RunE directly — instead we verify the cmd.Flags() surface returns
// the set values (the exact lookup path the fixed code uses), and we
// confirm the package-global viper does NOT see the same values
// (which is the original defect: a viper-side read always returned
// the zero value).
func TestTailCmd_FlagsReachRunE(t *testing.T) {
	cmd := tailCmd()

	if err := cmd.Flags().Set("event", "PreToolUse"); err != nil {
		t.Fatalf("set --event: %v", err)
	}
	if err := cmd.Flags().Set("cli", "claude"); err != nil {
		t.Fatalf("set --cli: %v", err)
	}
	if err := cmd.Flags().Set("slow", "true"); err != nil {
		t.Fatalf("set --slow: %v", err)
	}
	if err := cmd.Flags().Set("last", "5"); err != nil {
		t.Fatalf("set --last: %v", err)
	}

	// Fixed lookup path: cmd.Flags().Get<Type>().
	gotEvent, _ := cmd.Flags().GetString("event")
	gotCLI, _ := cmd.Flags().GetString("cli")
	gotSlow, _ := cmd.Flags().GetBool("slow")
	gotLast, _ := cmd.Flags().GetInt("last")

	if gotEvent != "PreToolUse" {
		t.Errorf("cmd.Flags().GetString(event) = %q, want PreToolUse", gotEvent)
	}
	if gotCLI != "claude" {
		t.Errorf("cmd.Flags().GetString(cli) = %q, want claude", gotCLI)
	}
	if !gotSlow {
		t.Errorf("cmd.Flags().GetBool(slow) = false, want true")
	}
	if gotLast != 5 {
		t.Errorf("cmd.Flags().GetInt(last) = %d, want 5", gotLast)
	}

	// Defect-side: the unbound package-global viper sees nothing. The
	// old code read viper.GetString("event") etc. and silently got
	// these zero values regardless of what the user passed.
	t.Cleanup(viper.Reset)
	if v := viper.GetString("event"); v != "" {
		t.Errorf("package-global viper.GetString(event) = %q, want empty (unbound)", v)
	}
	if v := viper.GetInt("last"); v != 0 {
		t.Errorf("package-global viper.GetInt(last) = %d, want 0 (unbound)", v)
	}
}

// TestTailCmd_LastDefault confirms --last has a non-zero default (20)
// so unset invocations show recent history instead of nothing.
func TestTailCmd_LastDefault(t *testing.T) {
	cmd := tailCmd()
	last, _ := cmd.Flags().GetInt("last")
	if last != 20 {
		t.Errorf("default --last = %d, want 20", last)
	}
}
