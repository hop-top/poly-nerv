package aider

import (
	"strings"
	"testing"
)

func merge(t *testing.T, in string) string {
	t.Helper()
	out, err := (&Adapter{}).MergeConfig([]byte(in))
	if err != nil {
		t.Fatalf("MergeConfig(%q): %v", in, err)
	}
	return string(out)
}

func TestMergeConfig_Golden(t *testing.T) {
	const f = `f(){ nervd --adapter aider `
	cases := []struct{ name, in, want string }{
		{"fresh", "",
			"# nerv hook integration\n" +
				"lint-cmd: nervd --adapter aider lint\n" +
				"test-cmd: nervd --adapter aider test\n" +
				"auto-lint: true\nauto-test: true\n"},
		{"unrelated keys kept", "model: gpt-4o\n# note\n",
			"model: gpt-4o\n# note\n\n# nerv hook integration\n" +
				"lint-cmd: nervd --adapter aider lint\n" +
				"test-cmd: nervd --adapter aider test\n" +
				"auto-lint: true\nauto-test: true\n"},
		{"wrap plain", "lint-cmd: flake8 --max-line-length 100\ntest-cmd: pytest -q\n",
			"lint-cmd: '" + f + `lint "$@" && flake8 --max-line-length 100 "$@"; }; f'` + "\n" +
				"test-cmd: '" + f + `test "$@" && pytest -q "$@"; }; f'` + "\n" +
				"\n# nerv hook integration\nauto-lint: true\nauto-test: true\n"},
		{"wrap quoted with colon-space and apostrophe", `lint-cmd: "sh -c 'echo a: b'"` + "\n" + `test-cmd: 'make test # all'` + "\n",
			"lint-cmd: '" + f + `lint "$@" && sh -c ''echo a: b'' "$@"; }; f'` + "\n" +
				"test-cmd: '" + f + `test "$@" && make test # all "$@"; }; f'` + "\n" +
				"\n# nerv hook integration\nauto-lint: true\nauto-test: true\n"},
		{"per-language", "lint-cmd: \"python: flake8 --select=E9\"\n",
			"lint-cmd: 'python: " + f + `lint "$@" && flake8 --select=E9 "$@"; }; f'` + "\n" +
				"auto-lint: true\nauto-test: true\n\n"}, // placeholder, fixed below
		{"legacy bare", "lint-cmd: nerv dispatch aider lint\ntest-cmd: nerv dispatch aider test\nauto-lint: true\nauto-test: true\n",
			"lint-cmd: nervd --adapter aider lint\ntest-cmd: nervd --adapter aider test\nauto-lint: true\nauto-test: true\n"},
		{"legacy chained", "lint-cmd: nerv dispatch aider lint && flake8\ntest-cmd: nerv dispatch aider test && pytest\nauto-lint: true\nauto-test: true\n",
			"lint-cmd: '" + f + `lint "$@" && flake8 "$@"; }; f'` + "\n" +
				"test-cmd: '" + f + `test "$@" && pytest "$@"; }; f'` + "\n" +
				"auto-lint: true\nauto-test: true\n"},
		{"auto flags forced on", "auto-lint: false\nauto-test: false\n",
			"auto-lint: true\nauto-test: true\n\n# nerv hook integration\n" +
				"lint-cmd: nervd --adapter aider lint\ntest-cmd: nervd --adapter aider test\n"},
	}
	// per-language: only lint-cmd present, so test-cmd + autos appended.
	cases[4].want = "lint-cmd: 'python: " + f + `lint "$@" && flake8 --select=E9 "$@"; }; f'` + "\n" +
		"\n# nerv hook integration\ntest-cmd: nervd --adapter aider test\nauto-lint: true\nauto-test: true\n"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := merge(t, tc.in); got != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

// TestMergeConfig_Idempotent: rerunning on any output is a no-op, and
// no legacy command or duplicate key survives.
func TestMergeConfig_Idempotent(t *testing.T) {
	inputs := []string{
		"", "model: x\n",
		"lint-cmd: flake8\ntest-cmd: pytest\n",
		"lint-cmd: \"python: flake8\"\n",
		"lint-cmd: nerv dispatch aider lint && flake8 # c\n",
		"lint-cmd: nerv dispatch aider lint && python: flake8\n",
	}
	for _, in := range inputs {
		first := merge(t, in)
		second := merge(t, first)
		if first != second {
			t.Errorf("not idempotent for %q:\n1: %s\n2: %s", in, first, second)
		}
		if strings.Contains(first, "nerv dispatch") {
			t.Errorf("legacy survived for %q:\n%s", in, first)
		}
		for _, k := range []string{"lint-cmd:", "test-cmd:", "auto-lint:", "auto-test:"} {
			if n := strings.Count(first, "\n"+k) + btoi(strings.HasPrefix(first, k)); n != 1 {
				t.Errorf("%q: %d x %s\n%s", in, n, k, first)
			}
		}
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestMergeConfig_LegacyPerLanguage: the old wrap glued a per-language
// original after the nerv command ("nerv ... && python: flake8"), which
// Aider cannot parse. The upgrade restores the language prefix.
func TestMergeConfig_LegacyPerLanguage(t *testing.T) {
	got := merge(t, "lint-cmd: nerv dispatch aider lint && python: flake8\n")
	want := `lint-cmd: 'python: f(){ nervd --adapter aider lint "$@" && flake8 "$@"; }; f'`
	if !strings.HasPrefix(got, want+"\n") {
		t.Fatalf("got:\n%s\nwant prefix:\n%s", got, want)
	}
}

// TestMergeConfig_RejectsLists: list-valued lint-cmd is refused and
// nothing is rewritten.
func TestMergeConfig_RejectsLists(t *testing.T) {
	for _, in := range []string{
		"lint-cmd:\n  - \"python: flake8\"\n",
		"lint-cmd: [\"python: flake8\", \"js: eslint\"]\n",
		"test-cmd: |\n  pytest\n",
	} {
		if out, err := (&Adapter{}).MergeConfig([]byte(in)); err == nil || out != nil {
			t.Errorf("%q: want error, got %q, %v", in, out, err)
		}
	}
}
