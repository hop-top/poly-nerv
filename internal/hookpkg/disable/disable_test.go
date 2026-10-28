package disable

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// seedProject writes a hooks.yaml into a tmp .nerv/ and returns the
// open Project. The yaml content is the caller-supplied seed; only
// the directory shape is created.
func seedProject(t *testing.T, hooksYAML string) *Project {
	t.Helper()
	root := t.TempDir()
	nervDir := filepath.Join(root, ".nerv")
	if err := os.MkdirAll(nervDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nervDir, "hooks.yaml"),
		[]byte(hooksYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := OpenProjectAt(root)
	if err != nil {
		t.Fatalf("OpenProjectAt: %v", err)
	}
	return p
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// fixtureBasic is a minimal hooks.yaml with three hooks. Comments are
// included so PreservesComments can verify they survive a write.
const fixtureBasic = `version: "1.0-draft"
# top-level comment
hooks:
  - name: foo  # inline comment on foo
    on: PreToolUse
    run: ./hooks/foo.sh
  - name: bar
    on: [PreToolUse, PostToolUse]
    run: ./hooks/bar.sh
  - name: baz
    on: SessionStart
    run: ./hooks/baz.sh
    timeout: 5
`

func TestOpenProject_WalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".nerv"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := OpenProjectAt(deep)
	if err != nil {
		t.Fatalf("OpenProjectAt deep: %v", err)
	}
	gotRoot, _ := filepath.EvalSymlinks(p.Root)
	wantRoot, _ := filepath.EvalSymlinks(root)
	if gotRoot != wantRoot {
		t.Fatalf("project root = %s, want %s", gotRoot, wantRoot)
	}
}

func TestOpenProject_ErrNotProject(t *testing.T) {
	root := t.TempDir()
	if _, err := OpenProjectAt(root); err != ErrNotProject {
		t.Fatalf("expected ErrNotProject, got %v", err)
	}
}

func TestDisableHooks_WritesFlag(t *testing.T) {
	p := seedProject(t, fixtureBasic)

	changed, unknown, err := p.DisableHooks([]string{"foo"})
	if err != nil {
		t.Fatalf("DisableHooks: %v", err)
	}
	if len(changed) != 1 || changed[0] != "foo" {
		t.Fatalf("changed=%v, want [foo]", changed)
	}
	if len(unknown) != 0 {
		t.Fatalf("unknown=%v, want []", unknown)
	}
	body := readFile(t, p.HooksYAML)
	if !strings.Contains(body, "disabled: true") {
		t.Fatalf("hooks.yaml missing disabled: true:\n%s", body)
	}
}

func TestDisableHooks_Idempotent(t *testing.T) {
	p := seedProject(t, fixtureBasic)

	if _, _, err := p.DisableHooks([]string{"foo"}); err != nil {
		t.Fatalf("first DisableHooks: %v", err)
	}
	body1 := readFile(t, p.HooksYAML)

	changed, _, err := p.DisableHooks([]string{"foo"})
	if err != nil {
		t.Fatalf("second DisableHooks: %v", err)
	}
	if len(changed) != 0 {
		t.Fatalf("second call should be no-op; changed=%v", changed)
	}
	body2 := readFile(t, p.HooksYAML)
	if body1 != body2 {
		t.Fatalf("file changed on idempotent re-call:\nbefore:\n%s\nafter:\n%s", body1, body2)
	}
}

func TestDisableHooks_UnknownNames(t *testing.T) {
	p := seedProject(t, fixtureBasic)

	changed, unknown, err := p.DisableHooks([]string{"nope", "foo"})
	if err != nil {
		t.Fatalf("DisableHooks: %v", err)
	}
	if len(changed) != 1 || changed[0] != "foo" {
		t.Fatalf("changed=%v, want [foo]", changed)
	}
	if len(unknown) != 1 || unknown[0] != "nope" {
		t.Fatalf("unknown=%v, want [nope]", unknown)
	}
}

func TestDisableHooks_PreservesComments(t *testing.T) {
	p := seedProject(t, fixtureBasic)

	if _, _, err := p.DisableHooks([]string{"foo"}); err != nil {
		t.Fatalf("DisableHooks: %v", err)
	}
	body := readFile(t, p.HooksYAML)
	// yaml.v3 Node API keeps both head and line comments — the
	// fixture's "# top-level comment" and "# inline comment on foo"
	// should both survive the write cycle.
	for _, want := range []string{"# top-level comment", "# inline comment on foo"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing comment %q in:\n%s", want, body)
		}
	}
}

func TestEnableHooks_StripsFlag(t *testing.T) {
	p := seedProject(t, fixtureBasic)
	if _, _, err := p.DisableHooks([]string{"foo"}); err != nil {
		t.Fatalf("DisableHooks: %v", err)
	}

	changed, _, err := p.EnableHooks([]string{"foo"})
	if err != nil {
		t.Fatalf("EnableHooks: %v", err)
	}
	if len(changed) != 1 || changed[0] != "foo" {
		t.Fatalf("changed=%v, want [foo]", changed)
	}
	body := readFile(t, p.HooksYAML)
	if strings.Contains(body, "disabled:") {
		t.Fatalf("disabled: key not stripped:\n%s", body)
	}
}

func TestEnable_Symmetric(t *testing.T) {
	// disable → enable round-trip restores file content (sans the
	// per-byte equality, since yaml.v3 may renormalise styling — we
	// instead verify semantic equality via re-parse).
	p := seedProject(t, fixtureBasic)
	orig := readFile(t, p.HooksYAML)

	if _, _, err := p.DisableHooks([]string{"foo", "bar"}); err != nil {
		t.Fatalf("DisableHooks: %v", err)
	}
	if _, _, err := p.EnableHooks([]string{"foo", "bar"}); err != nil {
		t.Fatalf("EnableHooks: %v", err)
	}
	after := readFile(t, p.HooksYAML)

	// Parse both as generic yaml and compare the structural shape.
	var a, b any
	if err := yaml.Unmarshal([]byte(orig), &a); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal([]byte(after), &b); err != nil {
		t.Fatal(err)
	}
	// Compare via yaml round-trip serialisation (canonicalises both).
	aBytes, _ := yaml.Marshal(a)
	bBytes, _ := yaml.Marshal(b)
	if string(aBytes) != string(bBytes) {
		t.Fatalf("symmetric round-trip diverged:\nbefore:\n%s\nafter:\n%s",
			aBytes, bBytes)
	}
}

func TestDisableEvents_Sidecar(t *testing.T) {
	p := seedProject(t, fixtureBasic)

	added, err := p.DisableEvents([]string{"PreToolUse", "PostToolUse"})
	if err != nil {
		t.Fatalf("DisableEvents: %v", err)
	}
	if len(added) != 2 {
		t.Fatalf("added=%v, want 2", added)
	}
	sc, err := p.LoadSidecar()
	if err != nil {
		t.Fatalf("LoadSidecar: %v", err)
	}
	if sc.Version != sidecarVersion {
		t.Fatalf("version=%q, want %q", sc.Version, sidecarVersion)
	}
	if len(sc.Events) != 2 {
		t.Fatalf("Events=%v, want 2", sc.Events)
	}
}

func TestDisableEvents_Idempotent(t *testing.T) {
	p := seedProject(t, fixtureBasic)

	if _, err := p.DisableEvents([]string{"PreToolUse"}); err != nil {
		t.Fatalf("first DisableEvents: %v", err)
	}
	added, err := p.DisableEvents([]string{"PreToolUse"})
	if err != nil {
		t.Fatalf("second DisableEvents: %v", err)
	}
	if len(added) != 0 {
		t.Fatalf("second call should be no-op; added=%v", added)
	}
}

func TestEnableEvents_Symmetric(t *testing.T) {
	p := seedProject(t, fixtureBasic)

	if _, err := p.DisableEvents([]string{"PreToolUse", "PostToolUse"}); err != nil {
		t.Fatalf("DisableEvents: %v", err)
	}
	removed, err := p.EnableEvents([]string{"PreToolUse"})
	if err != nil {
		t.Fatalf("EnableEvents: %v", err)
	}
	if len(removed) != 1 || removed[0] != "PreToolUse" {
		t.Fatalf("removed=%v, want [PreToolUse]", removed)
	}
	sc, _ := p.LoadSidecar()
	if len(sc.Events) != 1 || sc.Events[0] != "PostToolUse" {
		t.Fatalf("Events=%v, want [PostToolUse]", sc.Events)
	}
}

func TestDisableBindings_Sidecar(t *testing.T) {
	p := seedProject(t, fixtureBasic)

	pairs := []Binding{
		{Hook: "foo", Event: "PreToolUse"},
		{Hook: "bar", Event: "PostToolUse"},
	}
	added, err := p.DisableBindings(pairs)
	if err != nil {
		t.Fatalf("DisableBindings: %v", err)
	}
	if len(added) != 2 {
		t.Fatalf("added=%v, want 2", added)
	}

	// Idempotent re-add.
	again, err := p.DisableBindings(pairs)
	if err != nil {
		t.Fatalf("DisableBindings (again): %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("idempotent re-add should yield 0; got %v", again)
	}

	sc, err := p.LoadSidecar()
	if err != nil {
		t.Fatalf("LoadSidecar: %v", err)
	}
	if len(sc.Bindings) != 2 {
		t.Fatalf("Bindings=%v, want 2", sc.Bindings)
	}

	// Sidecar yaml shape check.
	body := readFile(t, p.DisabledYAML)
	for _, want := range []string{"version: 1.0-draft", "hook: foo", "event: PreToolUse"} {
		if !strings.Contains(body, want) {
			t.Fatalf("disabled.yaml missing %q:\n%s", want, body)
		}
	}
}

func TestEnableBindings_Symmetric(t *testing.T) {
	p := seedProject(t, fixtureBasic)

	pairs := []Binding{
		{Hook: "foo", Event: "PreToolUse"},
		{Hook: "bar", Event: "PostToolUse"},
	}
	if _, err := p.DisableBindings(pairs); err != nil {
		t.Fatalf("DisableBindings: %v", err)
	}
	removed, err := p.EnableBindings([]Binding{{Hook: "foo", Event: "PreToolUse"}})
	if err != nil {
		t.Fatalf("EnableBindings: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("removed=%v, want 1", removed)
	}
	sc, _ := p.LoadSidecar()
	if len(sc.Bindings) != 1 {
		t.Fatalf("Bindings=%v, want 1 left", sc.Bindings)
	}
	if sc.Bindings[0].Hook != "bar" {
		t.Fatalf("remaining binding=%v, want bar/PostToolUse", sc.Bindings[0])
	}
}

func TestListHookNames(t *testing.T) {
	p := seedProject(t, fixtureBasic)
	names, err := p.ListHookNames()
	if err != nil {
		t.Fatalf("ListHookNames: %v", err)
	}
	want := []string{"foo", "bar", "baz"}
	if len(names) != len(want) {
		t.Fatalf("names=%v, want %v", names, want)
	}
	for i, n := range names {
		if n != want[i] {
			t.Fatalf("names[%d]=%s, want %s", i, n, want[i])
		}
	}
}

func TestSidecar_RoundTrip(t *testing.T) {
	p := seedProject(t, fixtureBasic)

	if _, err := p.DisableEvents([]string{"PreToolUse"}); err != nil {
		t.Fatalf("DisableEvents: %v", err)
	}
	if _, err := p.DisableBindings([]Binding{{Hook: "foo", Event: "PostToolUse"}}); err != nil {
		t.Fatalf("DisableBindings: %v", err)
	}

	body := readFile(t, p.DisabledYAML)
	// Required fields present.
	for _, want := range []string{
		"version: 1.0-draft",
		"events:",
		"- PreToolUse",
		"bindings:",
		"hook: foo",
		"event: PostToolUse",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("disabled.yaml missing %q:\n%s", want, body)
		}
	}
}

func TestDisableHooks_EmptyNames(t *testing.T) {
	p := seedProject(t, fixtureBasic)
	changed, unknown, err := p.DisableHooks(nil)
	if err != nil {
		t.Fatalf("DisableHooks(nil): %v", err)
	}
	if len(changed) != 0 || len(unknown) != 0 {
		t.Fatalf("expected no-op for empty input; got changed=%v unknown=%v", changed, unknown)
	}
}
