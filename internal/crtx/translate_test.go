package crtx

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	kitbus "hop.top/kit/go/runtime/bus"

	"hop.top/nerv/internal/envelope"
)

// eventsSchemaURL matches the vendored schema's $id so $ref resolution
// stays self-contained (see testdata/README.md for provenance).
const eventsSchemaURL = "https://spec.hop.top/crtx/v0.1/events.schema.json"

// compilePayloadSchema compiles the $defs entry named def from the
// vendored events schema.
func compilePayloadSchema(t *testing.T, def string) *jsonschema.Schema {
	t.Helper()

	raw, err := os.ReadFile("testdata/events.schema.json")
	if err != nil {
		t.Fatalf("read vendored events schema: %v", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("decode vendored events schema: %v", err)
	}

	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource(eventsSchemaURL, doc); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	sch, err := c.Compile(eventsSchemaURL + "#/$defs/" + def)
	if err != nil {
		t.Fatalf("compile %s: %v", def, err)
	}
	return sch
}

// mustValidate marshals payload to JSON and validates the decoded
// instance against sch — the exact bytes a bus sink or recorder would
// observe.
func mustValidate(t *testing.T, sch *jsonschema.Schema, payload any) {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Fatalf("payload %s failed schema validation: %v", data, err)
	}
}

func hookEnv(event, eventID string, payload string) *envelope.Envelope {
	return &envelope.Envelope{
		NervVersion: "1.0",
		Event:       event,
		EventID:     eventID,
		Timestamp:   time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
		CLI: envelope.CLIInfo{
			Name:    "claude",
			Version: "2.1.0",
		},
		Session: envelope.SessionInfo{
			ID:  "9f36c8a2-6c5f-4a51-b2f4-1f2f3a4b5c6d",
			CWD: "/tmp/project",
		},
		Payload: json.RawMessage(payload),
	}
}

func singleEmission(t *testing.T, env *envelope.Envelope, wantTopic string) Emission {
	t.Helper()
	got := Translate(env)
	if len(got) != 1 {
		t.Fatalf("Translate(%s): got %d emissions, want 1", env.Event, len(got))
	}
	if got[0].Topic != wantTopic {
		t.Fatalf("Translate(%s): topic = %q, want %q", env.Event, got[0].Topic, wantTopic)
	}
	return got[0]
}

func TestTranslateNil(t *testing.T) {
	if got := Translate(nil); got != nil {
		t.Fatalf("Translate(nil) = %v, want nil", got)
	}
}

func TestTranslateSessionStart(t *testing.T) {
	env := hookEnv("SessionStart", "ev-001",
		`{"session_id":"9f36c8a2-6c5f-4a51-b2f4-1f2f3a4b5c6d","resumed":false,"cwd":"/tmp/project"}`)

	em := singleEmission(t, env, TopicSessionCreated)
	mustValidate(t, compilePayloadSchema(t, "SessionCreatedPayload"), em.Payload)

	p, ok := em.Payload.(SessionCreatedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want SessionCreatedPayload", em.Payload)
	}
	if p.EnvelopeID != env.Session.ID {
		t.Errorf("envelope_id = %q, want session id %q", p.EnvelopeID, env.Session.ID)
	}
	if p.Source.Kind != "claude-code" {
		t.Errorf("source.kind = %q, want normalized %q", p.Source.Kind, "claude-code")
	}
	if p.Source.Version != "2.1.0" {
		t.Errorf("source.version = %q, want %q", p.Source.Version, "2.1.0")
	}
}

func TestTranslateSessionStartUnknownCLIVersion(t *testing.T) {
	env := hookEnv("SessionStart", "ev-002", `{}`)
	env.CLI = envelope.CLIInfo{Name: "myagent"}

	em := singleEmission(t, env, TopicSessionCreated)
	mustValidate(t, compilePayloadSchema(t, "SessionCreatedPayload"), em.Payload)

	p := em.Payload.(SessionCreatedPayload)
	if p.Source.Kind != "myagent" {
		t.Errorf("source.kind = %q, want passthrough %q", p.Source.Kind, "myagent")
	}
	if p.Source.Version != "unknown" {
		t.Errorf("source.version = %q, want %q (schema minLength 1)", p.Source.Version, "unknown")
	}
}

func TestTranslateUserPromptSubmit(t *testing.T) {
	env := hookEnv("UserPromptSubmit", "ev-010",
		`{"prompt":"refactor the loader","turn_index":3}`)

	em := singleEmission(t, env, TopicUserReceived)
	mustValidate(t, compilePayloadSchema(t, "UserReceivedPayload"), em.Payload)

	p := em.Payload.(UserReceivedPayload)
	if p.TurnID != "ev-010" {
		t.Errorf("turn_id = %q, want event id", p.TurnID)
	}
	if len(p.Content) != 1 {
		t.Fatalf("content parts = %d, want 1", len(p.Content))
	}
	text, ok := p.Content[0].(TextPart)
	if !ok {
		t.Fatalf("content[0] type = %T, want TextPart", p.Content[0])
	}
	if text.Text != "refactor the loader" {
		t.Errorf("text = %q", text.Text)
	}
}

func TestTranslateToolCallCorrelation(t *testing.T) {
	pre := hookEnv("PreToolUse", "ev-020",
		`{"tool_name":"Bash","tool_input":{"command":"ls","timeout":5}}`)
	post := hookEnv("PostToolUse", "ev-021",
		`{"tool_name":"Bash","tool_input":{"timeout":5,"command":"ls"},"tool_response":{"stdout":"main.go"},"duration_ms":12}`)

	called := singleEmission(t, pre, TopicToolCalled)
	completed := singleEmission(t, post, TopicToolCompleted)

	mustValidate(t, compilePayloadSchema(t, "ToolCalledPayload"), called.Payload)
	mustValidate(t, compilePayloadSchema(t, "ToolCompletedPayload"), completed.Payload)

	cp := called.Payload.(ToolCalledPayload)
	rp := completed.Payload.(ToolCompletedPayload)

	if cp.CallID == "" {
		t.Fatal("tool.called call_id empty")
	}
	if cp.CallID != rp.CallID {
		t.Errorf("call_id mismatch: called %q vs completed %q (deterministic digest must pair Pre/Post regardless of key order)",
			cp.CallID, rp.CallID)
	}
	if cp.Name != "Bash" {
		t.Errorf("name = %q", cp.Name)
	}
	if rp.IsError {
		t.Error("is_error = true for successful PostToolUse")
	}
}

func TestTranslateToolCallDistinctInputsDistinctIDs(t *testing.T) {
	a := hookEnv("PreToolUse", "ev-022",
		`{"tool_name":"Bash","tool_input":{"command":"ls"}}`)
	b := hookEnv("PreToolUse", "ev-023",
		`{"tool_name":"Bash","tool_input":{"command":"pwd"}}`)

	ida := singleEmission(t, a, TopicToolCalled).Payload.(ToolCalledPayload).CallID
	idb := singleEmission(t, b, TopicToolCalled).Payload.(ToolCalledPayload).CallID
	if ida == idb {
		t.Errorf("distinct tool inputs produced identical call ids (%q)", ida)
	}
}

func TestTranslatePostToolUseFailure(t *testing.T) {
	env := hookEnv("PostToolUseFailure", "ev-030",
		`{"tool_name":"Bash","tool_input":{"command":"rm"},"error":"blocked by guardrails","exit_code":2}`)

	em := singleEmission(t, env, TopicToolCompleted)
	mustValidate(t, compilePayloadSchema(t, "ToolCompletedPayload"), em.Payload)

	p := em.Payload.(ToolCompletedPayload)
	if !p.IsError {
		t.Error("is_error = false, want true")
	}
	if p.Output != "blocked by guardrails" {
		t.Errorf("output = %v, want error string", p.Output)
	}
}

func TestTranslateStop(t *testing.T) {
	env := hookEnv("Stop", "ev-040", `{"stop_reason":"end_turn","turn_index":3}`)

	em := singleEmission(t, env, TopicAssistantEmitted)
	mustValidate(t, compilePayloadSchema(t, "AssistantEmittedPayload"), em.Payload)

	p := em.Payload.(AssistantEmittedPayload)
	if len(p.Content) != 1 {
		t.Fatalf("content parts = %d, want 1", len(p.Content))
	}
	ext, ok := p.Content[0].(ExtensionPart)
	if !ok {
		t.Fatalf("content[0] type = %T, want ExtensionPart", p.Content[0])
	}
	if ext["type"] != StopPartType {
		t.Errorf("extension part type = %v, want %q", ext["type"], StopPartType)
	}
	if ext["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v", ext["stop_reason"])
	}
}

func TestTranslateSubagentPair(t *testing.T) {
	start := hookEnv("SubagentStart", "ev-050",
		`{"agent_id":"sub-1","agent_type":"explore","parent_session_id":"9f36c8a2-6c5f-4a51-b2f4-1f2f3a4b5c6d"}`)
	stop := hookEnv("SubagentStop", "ev-051",
		`{"agent_id":"sub-1","agent_type":"explore","exit_code":1,"duration_ms":900}`)

	nested := singleEmission(t, start, TopicSessionNested)
	returned := singleEmission(t, stop, TopicSessionReturned)

	mustValidate(t, compilePayloadSchema(t, "SessionNestedPayload"), nested.Payload)
	mustValidate(t, compilePayloadSchema(t, "SessionReturnedPayload"), returned.Payload)

	np := nested.Payload.(SessionNestedPayload)
	rp := returned.Payload.(SessionReturnedPayload)

	if np.ChildID != "sub-1" {
		t.Errorf("nested child_id = %q", np.ChildID)
	}
	if np.ParentID != "9f36c8a2-6c5f-4a51-b2f4-1f2f3a4b5c6d" {
		t.Errorf("nested parent_id = %q", np.ParentID)
	}
	if np.CallID == "" || np.CallID != rp.CallID {
		t.Errorf("synthetic call ids must pair: nested %q vs returned %q", np.CallID, rp.CallID)
	}
	if !rp.IsError {
		t.Error("returned is_error = false, want true for exit_code 1")
	}
}

func TestTranslateVendorTopics(t *testing.T) {
	cases := []struct {
		event   string
		payload string
		topic   string
	}{
		{"SessionEnd", `{"session_id":"s","exit_code":0,"duration_ms":100}`, TopicVendorSessionEnded},
		{"StopFailure", `{"error":"api timeout","error_code":"529","turn_index":2}`, TopicVendorTurnFailed},
		{"Notification", `{"message":"waiting on input","level":"info"}`, TopicVendorNotificationEmitted},
		{"PermissionRequest", `{"tool_name":"Bash","tool_input":{},"permission_type":"execute"}`, TopicVendorPermissionRequested},
		{"PermissionDenied", `{"tool_name":"Bash","tool_input":{},"denied_by":"user","reason":"nope"}`, TopicVendorPermissionDenied},
		{"TaskCreated", `{"task_id":"1","title":"t"}`, TopicVendorTaskCreated},
		{"TaskCompleted", `{"task_id":"1","title":"t","result":"ok"}`, TopicVendorTaskCompleted},
	}
	for _, tc := range cases {
		env := hookEnv(tc.event, "ev-"+tc.event, tc.payload)
		em := singleEmission(t, env, tc.topic)

		p, ok := em.Payload.(VendorPayload)
		if !ok {
			t.Fatalf("%s payload type = %T, want VendorPayload", tc.event, em.Payload)
		}
		if p.EnvelopeID != env.Session.ID {
			t.Errorf("%s envelope_id = %q", tc.event, p.EnvelopeID)
		}
		if p.Event != tc.event {
			t.Errorf("vendor payload event = %q, want %q", p.Event, tc.event)
		}
		if p.EventID != env.EventID {
			t.Errorf("%s event_id = %q", tc.event, p.EventID)
		}
		if p.OccurredAt == "" {
			t.Errorf("%s occurred_at empty", tc.event)
		}
		if string(p.Payload) != tc.payload {
			t.Errorf("%s payload not preserved verbatim: %s", tc.event, p.Payload)
		}
	}
}

func TestTranslateDroppedEvents(t *testing.T) {
	dropped := []string{
		"TeammateIdle", "InstructionsLoaded", "ConfigChange", "CwdChanged",
		"FileChanged", "WorktreeCreate", "WorktreeRemove", "PreCompact",
		"PostCompact", "Elicitation", "ElicitationResult",
		"BeforeToolSelection", "LspDiagnostics", "SessionDiff",
		"SessionStatus", "MemoryRead", "MemoryWrite", "TotallyUnknownEvent",
	}
	for _, ev := range dropped {
		env := hookEnv(ev, "ev-x", `{}`)
		if got := Translate(env); len(got) != 0 {
			t.Errorf("Translate(%s) = %d emissions, want 0 (dropped per ADR)", ev, len(got))
		}
	}
}

// TestEmittedTopicsSatisfyBusGrammar locks every topic constant to the
// kit/bus 4-segment publish grammar so warn-mode validation stays
// silent in production.
func TestEmittedTopicsSatisfyBusGrammar(t *testing.T) {
	for _, topic := range AllTopics() {
		if err := kitbus.Validate(kitbus.Topic(topic)); err != nil {
			t.Errorf("topic %q rejected by bus grammar: %v", topic, err)
		}
	}
}

// TestCanonicalTopicNames pins the canonical constants to the exact
// names from the crtx v0.1 events spec §3.
func TestCanonicalTopicNames(t *testing.T) {
	want := map[string]string{
		TopicUserReceived:     "crtx.turn.user.received",
		TopicAssistantEmitted: "crtx.turn.assistant.emitted",
		TopicToolCalled:       "crtx.turn.tool.called",
		TopicToolCompleted:    "crtx.turn.tool.completed",
		TopicThinkingEmitted:  "crtx.turn.thinking.emitted",
		TopicSystemInjected:   "crtx.turn.system.injected",
		TopicSessionCreated:   "crtx.session.envelope.created",
		TopicSessionForked:    "crtx.session.envelope.forked",
		TopicSessionNested:    "crtx.session.envelope.nested",
		TopicSessionReturned:  "crtx.session.envelope.returned",
	}
	for got, expect := range want {
		if got != expect {
			t.Errorf("canonical topic constant = %q, want %q", got, expect)
		}
	}
}

// TestVendorTopicsUseVendorPrefix guards the events-spec §5 rule: no
// invented crtx.* topics; extensions live under x.nerv.*.
func TestVendorTopicsUseVendorPrefix(t *testing.T) {
	canonical := map[string]bool{
		TopicUserReceived: true, TopicAssistantEmitted: true,
		TopicToolCalled: true, TopicToolCompleted: true,
		TopicThinkingEmitted: true, TopicSystemInjected: true,
		TopicSessionCreated: true, TopicSessionForked: true,
		TopicSessionNested: true, TopicSessionReturned: true,
	}
	for _, topic := range AllTopics() {
		if canonical[topic] {
			continue
		}
		if len(topic) < 7 || topic[:7] != "x.nerv." {
			t.Errorf("non-canonical topic %q must carry the x.nerv. vendor prefix", topic)
		}
	}
}
