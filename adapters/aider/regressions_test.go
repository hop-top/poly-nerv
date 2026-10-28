package aider

import (
	"strings"
	"testing"
)

// --- Issue 8: duplicate lint-cmd/test-cmd keys ---

func TestRegression_MergeConfig_DuplicateLintCmd(t *testing.T) {
	existing := []byte("lint-cmd: flake8\ntest-cmd: pytest\n")

	a := &Adapter{}
	merged, err := a.MergeConfig(existing)
	if err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}
	content := string(merged)

	// Must NOT have duplicate lint-cmd lines.
	count := strings.Count(content, "lint-cmd:")
	if count > 1 {
		t.Errorf("found %d 'lint-cmd:' lines, want 1:\n%s",
			count, content)
	}

	// Must NOT have duplicate test-cmd lines.
	count = strings.Count(content, "test-cmd:")
	if count > 1 {
		t.Errorf("found %d 'test-cmd:' lines, want 1:\n%s",
			count, content)
	}

	// Existing command must be preserved (wrapped with nerv).
	if !strings.Contains(content, "flake8") {
		t.Error("merged config lost existing lint command 'flake8'")
	}
	if !strings.Contains(content, "pytest") {
		t.Error("merged config lost existing test command 'pytest'")
	}

	// nerv dispatch must be present.
	if !strings.Contains(content, "nerv dispatch aider") {
		t.Error("merged config missing nerv dispatch")
	}
}

func TestRegression_MergeConfig_NoExistingKeys(t *testing.T) {
	existing := []byte("# just a comment\n")

	a := &Adapter{}
	merged, err := a.MergeConfig(existing)
	if err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}
	content := string(merged)

	// Should have exactly one lint-cmd and one test-cmd.
	if strings.Count(content, "lint-cmd:") != 1 {
		t.Errorf("expected 1 lint-cmd line:\n%s", content)
	}
	if strings.Count(content, "test-cmd:") != 1 {
		t.Errorf("expected 1 test-cmd line:\n%s", content)
	}
}
