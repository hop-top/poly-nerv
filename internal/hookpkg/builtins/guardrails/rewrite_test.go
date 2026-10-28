package guardrails

import "testing"

func TestStripFlag_MidString(t *testing.T) {
	got := stripFlag("git commit --no-verify -m wip", "--no-verify")
	want := "git commit -m wip"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripFlag_TrailingFlag(t *testing.T) {
	got := stripFlag("git commit -m wip --no-verify", "--no-verify")
	want := "git commit -m wip"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripFlag_NotPresent(t *testing.T) {
	got := stripFlag("git commit -m wip", "--no-verify")
	if got != "git commit -m wip" {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestStripFlag_DoesNotMatchSubstring(t *testing.T) {
	// --no-verify-tag must not be touched when stripping --no-verify.
	got := stripFlag("git commit --no-verify-tag -m wip", "--no-verify")
	if got != "git commit --no-verify-tag -m wip" {
		t.Errorf("substring match: got %q", got)
	}
}

func TestReplaceToken_WholeToken(t *testing.T) {
	got := replaceToken("git push --force origin main", "--force", "--force-with-lease")
	want := "git push --force-with-lease origin main"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReplaceToken_SkipsSubstring(t *testing.T) {
	// --force inside --force-with-lease must NOT match.
	got := replaceToken("git push --force-with-lease origin main", "--force", "--FORCE")
	if got != "git push --force-with-lease origin main" {
		t.Errorf("substring matched: got %q", got)
	}
}

func TestPrependArgs_BasicInsert(t *testing.T) {
	got := prependArgs("git push origin x", []string{"--dry-run"})
	want := "git --dry-run push origin x"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrependArgs_MultipleArgs(t *testing.T) {
	got := prependArgs("git push origin x", []string{"-c", "color=never"})
	want := "git -c color=never push origin x"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPrependArgs_EmptyCmd(t *testing.T) {
	if prependArgs("", []string{"--x"}) != "" {
		t.Error("empty cmd should pass through")
	}
}

func TestPrependArgs_EmptyArgs(t *testing.T) {
	if prependArgs("git push", nil) != "git push" {
		t.Error("nil args should pass through")
	}
}

func TestApplyRewrites_ChainedStrategies(t *testing.T) {
	// strip_flag then replace_token should compose.
	cmd := "git commit --no-verify --force -m wip"
	rewrites := []RewriteStrategy{
		{Strategy: "strip_flag", Flag: "--no-verify"},
		{Strategy: "replace_token", From: "--force", To: "--force-with-lease"},
	}
	got, changed := applyRewrites(cmd, rewrites)
	want := "git commit --force-with-lease -m wip"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if !changed {
		t.Error("changed=false despite two effective rewrites")
	}
}

func TestApplyRewrites_NoOpReportsFalse(t *testing.T) {
	rewrites := []RewriteStrategy{{Strategy: "strip_flag", Flag: "--missing"}}
	got, changed := applyRewrites("git commit -m wip", rewrites)
	if got != "git commit -m wip" {
		t.Errorf("expected unchanged cmd, got %q", got)
	}
	if changed {
		t.Error("changed=true despite no-op strategies")
	}
}
