package builtins

import (
	"reflect"
	"testing"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// withRegistry swaps All for the duration of a test and restores it on
// cleanup. Tests use this instead of mutating All directly so global
// state is never observed by sibling tests.
func withRegistry(t *testing.T, defs []BuiltinDef) {
	t.Helper()
	saved := All
	All = defs
	t.Cleanup(func() { All = saved })
}

// stubFunc satisfies BuiltinFunc without exercising any real logic.
func stubFunc(_ *envelope.Envelope, _ BuiltinRuntimeCfg) (merger.Decision, error) {
	return merger.Decision{Action: merger.Allow}, nil
}

func TestByName_Present(t *testing.T) {
	def := BuiltinDef{
		Name:          "open-questions",
		SchemaVersion: 1,
		Events:        []string{"UserPromptSubmit", "Stop"},
		Func:          stubFunc,
	}
	withRegistry(t, []BuiltinDef{def})

	got, ok := ByName("open-questions")
	if !ok {
		t.Fatal("ByName returned ok=false for registered builtin")
	}
	if got.Name != def.Name {
		t.Errorf("ByName Name = %q, want %q", got.Name, def.Name)
	}
	if got.SchemaVersion != def.SchemaVersion {
		t.Errorf("ByName SchemaVersion = %d, want %d", got.SchemaVersion, def.SchemaVersion)
	}
	if !reflect.DeepEqual(got.Events, def.Events) {
		t.Errorf("ByName Events = %v, want %v", got.Events, def.Events)
	}
}

func TestByName_Absent(t *testing.T) {
	withRegistry(t, []BuiltinDef{
		{Name: "alpha", Func: stubFunc},
	})

	got, ok := ByName("missing")
	if ok {
		t.Fatal("ByName returned ok=true for unregistered builtin")
	}
	if got.Name != "" {
		t.Errorf("ByName returned non-zero def for missing name: %+v", got)
	}
}

func TestByName_EmptyRegistry(t *testing.T) {
	withRegistry(t, []BuiltinDef{})

	if _, ok := ByName("anything"); ok {
		t.Fatal("ByName returned ok=true against empty registry")
	}
}

func TestInject_AppendsAfterUserHooks(t *testing.T) {
	withRegistry(t, []BuiltinDef{
		{
			Name:   "open-questions",
			Events: []string{"UserPromptSubmit", "Stop"},
			Func:   stubFunc,
		},
	})

	cfg := &config.HooksConfig{}
	user := config.HookEntry{Name: "my-hook", On: "UserPromptSubmit"}
	byEvent := map[string][]config.HookEntry{
		"UserPromptSubmit": {user},
	}

	Inject(cfg, byEvent)

	// UserPromptSubmit bucket: user first, builtin second.
	got := byEvent["UserPromptSubmit"]
	if len(got) != 2 {
		t.Fatalf("UserPromptSubmit bucket len = %d, want 2", len(got))
	}
	if got[0].Name != "my-hook" {
		t.Errorf("first entry Name = %q, want %q (user hook must precede builtin)", got[0].Name, "my-hook")
	}
	if got[0].Source != "" {
		t.Errorf("user hook Source = %q, want empty (zero-value)", got[0].Source)
	}
	if got[1].Name != "builtin:open-questions" {
		t.Errorf("second entry Name = %q, want %q", got[1].Name, "builtin:open-questions")
	}
	if got[1].Source != "builtin" {
		t.Errorf("builtin entry Source = %q, want %q", got[1].Source, "builtin")
	}
	if got[1].On != "UserPromptSubmit" {
		t.Errorf("builtin entry On = %v, want %q", got[1].On, "UserPromptSubmit")
	}

	// Stop bucket: empty before Inject; builtin alone after.
	stop := byEvent["Stop"]
	if len(stop) != 1 {
		t.Fatalf("Stop bucket len = %d, want 1", len(stop))
	}
	if stop[0].Name != "builtin:open-questions" || stop[0].Source != "builtin" {
		t.Errorf("Stop bucket entry = %+v, want builtin:open-questions/builtin", stop[0])
	}
}

func TestInject_MultipleBuiltinsPreserveOrder(t *testing.T) {
	withRegistry(t, []BuiltinDef{
		{Name: "first", Events: []string{"PreToolUse"}, Func: stubFunc},
		{Name: "second", Events: []string{"PreToolUse"}, Func: stubFunc},
	})

	byEvent := map[string][]config.HookEntry{}
	Inject(&config.HooksConfig{}, byEvent)

	got := byEvent["PreToolUse"]
	if len(got) != 2 {
		t.Fatalf("PreToolUse bucket len = %d, want 2", len(got))
	}
	if got[0].Name != "builtin:first" {
		t.Errorf("got[0].Name = %q, want %q", got[0].Name, "builtin:first")
	}
	if got[1].Name != "builtin:second" {
		t.Errorf("got[1].Name = %q, want %q", got[1].Name, "builtin:second")
	}
}

func TestInject_NoOpWhenAllEmpty(t *testing.T) {
	withRegistry(t, []BuiltinDef{})

	user := config.HookEntry{Name: "my-hook", On: "Stop"}
	byEvent := map[string][]config.HookEntry{
		"Stop": {user},
	}

	Inject(&config.HooksConfig{}, byEvent)

	got := byEvent["Stop"]
	if len(got) != 1 {
		t.Fatalf("bucket len = %d, want 1 (Inject must not add entries when All is empty)", len(got))
	}
	if got[0].Name != "my-hook" {
		t.Errorf("user hook overwritten: got %q", got[0].Name)
	}
}

// TestAll_DefaultEmpty pins the contract that no built-ins ship in the
// binary at this task's commit. Built-ins land in later changes
// (open-questions, etc.).
func TestAll_DefaultEmpty(t *testing.T) {
	if len(All) != 0 {
		t.Fatalf("All has %d entries, want 0 (no built-ins registered yet)", len(All))
	}
}
