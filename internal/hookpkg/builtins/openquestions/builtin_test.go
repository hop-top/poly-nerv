package openquestions

import (
	"reflect"
	"testing"

	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

func TestDef_Identity(t *testing.T) {
	if Def.Name != "open-questions" {
		t.Fatalf("Def.Name = %q, want %q", Def.Name, "open-questions")
	}
	if Def.SchemaVersion != 1 {
		t.Fatalf("Def.SchemaVersion = %d, want 1", Def.SchemaVersion)
	}
	if Def.Func == nil {
		t.Fatal("Def.Func is nil; expected Handle wired in")
	}
}

func TestDef_EventsMatchSpec(t *testing.T) {
	want := []string{
		"UserPromptSubmit",
		"PreToolUse",
		"CwdChanged",
		"Stop",
		"SessionEnd",
		"PreCompact",
	}
	if !reflect.DeepEqual(Def.Events, want) {
		t.Fatalf("Def.Events = %v, want %v", Def.Events, want)
	}
}

func TestHandle_ReturnsAllow_NilEnvelope(t *testing.T) {
	dec, err := Handle(nil, StubRuntimeCfg{})
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if dec.Action != merger.Allow {
		t.Fatalf("Handle.Action = %s, want allow", dec.Action)
	}
	if dec.Message != "" {
		t.Fatalf("Handle.Message = %q, want empty", dec.Message)
	}
	if dec.Rewrite != nil {
		t.Fatalf("Handle.Rewrite = %v, want nil", dec.Rewrite)
	}
}

func TestHandle_ReturnsAllow_PerEvent(t *testing.T) {
	cfg := StubRuntimeCfg{
		UserCfg:  map[string]any{},
		StateDir: "/tmp/nerv-state",
		InboxDir: "/tmp/nerv-inbox",
	}

	for _, ev := range Def.Events {
		ev := ev
		t.Run(ev, func(t *testing.T) {
			env := &envelope.Envelope{Event: ev}
			dec, err := Handle(env, cfg)
			if err != nil {
				t.Fatalf("Handle(%s) error: %v", ev, err)
			}
			if dec.Action != merger.Allow {
				t.Fatalf("Handle(%s).Action = %s, want allow", ev, dec.Action)
			}
		})
	}
}

func TestHandle_DoesNotPanic_OnArbitraryEvent(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Handle panicked: %v", r)
		}
	}()
	env := &envelope.Envelope{Event: "TotallyMadeUpEvent"}
	if _, err := Handle(env, StubRuntimeCfg{}); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
}

func TestDef_FuncDispatchesToHandle(t *testing.T) {
	env := &envelope.Envelope{Event: "Stop"}
	dec, err := Def.Func(env, StubRuntimeCfg{})
	if err != nil {
		t.Fatalf("Def.Func error: %v", err)
	}
	if dec.Action != merger.Allow {
		t.Fatalf("Def.Func.Action = %s, want allow", dec.Action)
	}
}
