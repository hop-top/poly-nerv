package events

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAllEvents(t *testing.T) {
	all := AllEvents()
	if got := len(all); got != 36 {
		t.Fatalf("AllEvents() returned %d events, want 36", got)
	}

	// Verify it returns a copy, not the backing slice.
	all[0] = "mutated"
	if AllEvents()[0] == "mutated" {
		t.Fatal("AllEvents() returned backing slice, not a copy")
	}
}

func TestIsValid_Known(t *testing.T) {
	if !IsValid("SessionStart") {
		t.Error("IsValid(\"SessionStart\") = false, want true")
	}
}

func TestIsValid_Unknown(t *testing.T) {
	if IsValid("FakeEvent") {
		t.Error("IsValid(\"FakeEvent\") = true, want false")
	}
}

func TestCategory(t *testing.T) {
	if got := Category("PreToolUse"); got != "tool" {
		t.Errorf("Category(\"PreToolUse\") = %q, want \"tool\"", got)
	}
}

func TestIsBlocking(t *testing.T) {
	if !IsBlocking("PreToolUse") {
		t.Error("IsBlocking(\"PreToolUse\") = false, want true")
	}
	if IsBlocking("SessionStart") {
		t.Error("IsBlocking(\"SessionStart\") = true, want false")
	}
}

func TestOrigin(t *testing.T) {
	cases := []struct {
		event Event
		want  string
	}{
		{"SessionStart", "native"},
		{"PreToolUse", "native"},
		{"BeforeToolSelection", "extension"},
		{"LspDiagnostics", "extension"},
		{"SessionDiff", "extension"},
		{"SessionStatus", "extension"},
		{"FakeEvent", ""},
	}
	for _, tc := range cases {
		if got := Origin(tc.event); got != tc.want {
			t.Errorf("Origin(%q) = %q, want %q", tc.event, got, tc.want)
		}
	}
}

// Bus / inbox-message lifecycle (kit-bus phase 1) — see
// docs/architecture/kit-bus-adoption.md §5 + US-0007 / US-0018.
//
// These tests guard against accidental rename / removal of the four
// canonical inbox-message events and pin their payload contracts so
// downstream subscribers (ctxt, tlc, agent harnesses) don't drift.

func TestInboxMessageEvents_Registered(t *testing.T) {
	events := []Event{
		EventInboxMessageAdded,
		EventInboxMessageUpdated,
		EventInboxMessagePromoted,
		EventInboxMessageDropped,
	}
	for _, e := range events {
		if !IsValid(e) {
			t.Errorf("IsValid(%q) = false, want true", e)
		}
		if got := Category(e); got != "bus" {
			t.Errorf("Category(%q) = %q, want \"bus\"", e, got)
		}
		// Bus events MUST be non-blocking — they are side-channel
		// notifications, never on the dispatch hot path.
		if IsBlocking(e) {
			t.Errorf("IsBlocking(%q) = true, want false", e)
		}
	}
}

func TestIsNative_IsExtension_IsDerived(t *testing.T) {
	if !IsNative("SessionStart") {
		t.Error("IsNative(SessionStart) = false, want true")
	}
	if IsExtension("SessionStart") {
		t.Error("IsExtension(SessionStart) = true, want false")
	}
	if !IsExtension("LspDiagnostics") {
		t.Error("IsExtension(LspDiagnostics) = false, want true")
	}
	if IsNative("LspDiagnostics") {
		t.Error("IsNative(LspDiagnostics) = true, want false")
	}
	if !IsDerived("MemoryRead") {
		t.Error("IsDerived(MemoryRead) = false, want true")
	}
	if !IsDerived("MemoryWrite") {
		t.Error("IsDerived(MemoryWrite) = false, want true")
	}
	if IsNative("MemoryRead") {
		t.Error("IsNative(MemoryRead) = true, want false")
	}
	if IsDerived("SessionStart") {
		t.Error("IsDerived(SessionStart) = true, want false")
	}
}

func TestDerivedCount(t *testing.T) {
	got := 0
	for _, e := range AllEvents() {
		if IsDerived(e) {
			got++
		}
	}
	if got != 2 {
		t.Errorf("derived event count = %d, want 2", got)
	}
}

func TestMemoryEvents_Category(t *testing.T) {
	if got := Category("MemoryRead"); got != "memory" {
		t.Errorf("Category(MemoryRead) = %q, want \"memory\"", got)
	}
	if got := Category("MemoryWrite"); got != "memory" {
		t.Errorf("Category(MemoryWrite) = %q, want \"memory\"", got)
	}
}

func TestExtensionCount(t *testing.T) {
	got := 0
	for _, e := range AllEvents() {
		if IsExtension(e) {
			got++
		}
	}
	if got != 4 {
		t.Errorf("extension event count = %d, want 4", got)
	}
}

func TestInboxMessageEvents_WireFormat(t *testing.T) {
	// US-0018 AC: item events follow <container>_<item>_<verb>.
	cases := map[Event]string{
		EventInboxMessageAdded:    "inbox_message_added",
		EventInboxMessageUpdated:  "inbox_message_updated",
		EventInboxMessagePromoted: "inbox_message_promoted",
		EventInboxMessageDropped:  "inbox_message_dropped",
	}
	for got, want := range cases {
		if string(got) != want {
			t.Errorf("event wire name = %q, want %q", got, want)
		}
	}
}

// payloadField mirrors spec/events.yaml shape for spec-loading tests.
type payloadField struct {
	Field    string   `yaml:"field"`
	Type     string   `yaml:"type"`
	Required bool     `yaml:"required"`
	Format   string   `yaml:"format"`
	Example  string   `yaml:"example"`
	Enum     []string `yaml:"enum"`
}

type eventDef struct {
	Name     string         `yaml:"name"`
	Category string         `yaml:"category"`
	Payload  []payloadField `yaml:"payload"`
}

type eventsSpec struct {
	Events []eventDef `yaml:"events"`
}

// loadSpec parses spec/events.yaml from the repo root by walking up
// from the test's cwd until go.mod is found.
func loadSpec(t *testing.T) eventsSpec {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	root := dir
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatalf("could not locate go.mod from %s", dir)
		}
		root = parent
	}
	data, err := os.ReadFile(filepath.Join(root, "spec", "events.yaml"))
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var spec eventsSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	return spec
}

func findEvent(t *testing.T, spec eventsSpec, name string) eventDef {
	t.Helper()
	for _, e := range spec.Events {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("event %q not found in spec/events.yaml", name)
	return eventDef{}
}

func payloadFields(e eventDef) map[string]payloadField {
	out := make(map[string]payloadField, len(e.Payload))
	for _, f := range e.Payload {
		out[f.Field] = f
	}
	return out
}

func TestInboxMessageAdded_PayloadShape(t *testing.T) {
	spec := loadSpec(t)
	ev := findEvent(t, spec, "inbox_message_added")
	fields := payloadFields(ev)

	want := []string{
		"schema_version", "msg_id", "agent_id", "inbox_id",
		"source_session_id", "kind", "ts", "payload_ref",
	}
	for _, name := range want {
		if _, ok := fields[name]; !ok {
			t.Errorf("inbox_message_added.payload missing field %q", name)
		}
	}
	// msg_id is the canonical correlation key — must be required.
	if !fields["msg_id"].Required {
		t.Error("inbox_message_added.msg_id.required = false, want true")
	}
	if got := fields["ts"].Format; got != "rfc3339" {
		t.Errorf("inbox_message_added.ts.format = %q, want \"rfc3339\"", got)
	}
}

func TestInboxMessageUpdated_PayloadShape(t *testing.T) {
	spec := loadSpec(t)
	ev := findEvent(t, spec, "inbox_message_updated")
	fields := payloadFields(ev)

	for _, name := range []string{"schema_version", "msg_id", "fields_changed", "ts"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("inbox_message_updated.payload missing field %q", name)
		}
	}
	if got := fields["fields_changed"].Type; got != "array" {
		t.Errorf("fields_changed.type = %q, want \"array\"", got)
	}
	if !fields["msg_id"].Required {
		t.Error("inbox_message_updated.msg_id.required = false, want true")
	}
}

func TestInboxMessagePromoted_PayloadShape(t *testing.T) {
	spec := loadSpec(t)
	ev := findEvent(t, spec, "inbox_message_promoted")
	fields := payloadFields(ev)

	for _, name := range []string{"schema_version", "msg_id", "target", "target_id", "ts"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("inbox_message_promoted.payload missing field %q", name)
		}
	}
	if !fields["msg_id"].Required {
		t.Error("inbox_message_promoted.msg_id.required = false, want true")
	}
	wantEnum := map[string]bool{"tlc-task": true, "ctxt-object": true, "other": true}
	got := fields["target"].Enum
	if len(got) != len(wantEnum) {
		t.Fatalf("target.enum = %v, want %v", got, wantEnum)
	}
	for _, v := range got {
		if !wantEnum[v] {
			t.Errorf("target.enum has unexpected value %q", v)
		}
	}
}

func TestInboxMessageDropped_PayloadShape(t *testing.T) {
	spec := loadSpec(t)
	ev := findEvent(t, spec, "inbox_message_dropped")
	fields := payloadFields(ev)

	for _, name := range []string{"schema_version", "msg_id", "reason", "ts"} {
		if _, ok := fields[name]; !ok {
			t.Errorf("inbox_message_dropped.payload missing field %q", name)
		}
	}
	if !fields["msg_id"].Required {
		t.Error("inbox_message_dropped.msg_id.required = false, want true")
	}
	wantEnum := map[string]bool{"user-deleted": true, "ttl-expired": true, "duplicate": true}
	got := fields["reason"].Enum
	if len(got) != len(wantEnum) {
		t.Fatalf("reason.enum = %v, want %v", got, wantEnum)
	}
	for _, v := range got {
		if !wantEnum[v] {
			t.Errorf("reason.enum has unexpected value %q", v)
		}
	}
}

// TestInboxContainerEvents_NotRegistered guards US-0018: container
// lifecycle events (`inbox_created`, `inbox_destroyed`) are RESERVED
// and MUST NOT be added under the `bus` category until a separate
// ADR lands. If this test starts failing, audit spec/events.yaml.
func TestInboxContainerEvents_NotRegistered(t *testing.T) {
	for _, reserved := range []Event{"inbox_created", "inbox_destroyed"} {
		if IsValid(reserved) {
			t.Errorf("reserved event %q is registered; see US-0018", reserved)
		}
	}
}
