package crtx

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The vendored crtx v0.1 schemas double as the runtime trust boundary:
// the consumer validates every inbound envelope / canonical event
// payload against the exact bytes the producer tests pin. See
// docs/architecture/crtx-consumer.md §"Trust and validation boundary".

//go:embed testdata/envelope.schema.json
var envelopeSchemaJSON []byte

//go:embed testdata/events.schema.json
var eventsSchemaJSON []byte

const (
	envelopeSchemaID = "https://spec.hop.top/crtx/v0.1/envelope.schema.json"
	eventsSchemaID   = "https://spec.hop.top/crtx/v0.1/events.schema.json"
)

// payloadDefs maps each canonical topic to its $defs entry in the
// events schema. Extension (x.*) topics are absent by design: their
// payloads are caller-defined (events spec §5) and cannot be
// schema-validated here.
var payloadDefs = map[string]string{
	TopicUserReceived:     "UserReceivedPayload",
	TopicAssistantEmitted: "AssistantEmittedPayload",
	TopicToolCalled:       "ToolCalledPayload",
	TopicToolCompleted:    "ToolCompletedPayload",
	TopicThinkingEmitted:  "ThinkingEmittedPayload",
	TopicSystemInjected:   "SystemInjectedPayload",
	TopicSessionCreated:   "SessionCreatedPayload",
	TopicSessionForked:    "SessionForkedPayload",
	TopicSessionNested:    "SessionNestedPayload",
	TopicSessionReturned:  "SessionReturnedPayload",
}

var (
	schemaOnce     sync.Once
	schemaInitErr  error
	envelopeSchema *jsonschema.Schema
	payloadSchemas map[string]*jsonschema.Schema
)

// compileSchemas compiles the embedded schemas once. A compile failure
// (impossible unless the vendored files are corrupted) is remembered
// and surfaced by every validation call.
func compileSchemas() {
	envDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(envelopeSchemaJSON))
	if err != nil {
		schemaInitErr = fmt.Errorf("crtx: decode vendored envelope schema: %w", err)
		return
	}
	evDoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(eventsSchemaJSON))
	if err != nil {
		schemaInitErr = fmt.Errorf("crtx: decode vendored events schema: %w", err)
		return
	}

	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource(envelopeSchemaID, envDoc); err != nil {
		schemaInitErr = fmt.Errorf("crtx: add envelope schema: %w", err)
		return
	}
	if err := c.AddResource(eventsSchemaID, evDoc); err != nil {
		schemaInitErr = fmt.Errorf("crtx: add events schema: %w", err)
		return
	}

	envelopeSchema, err = c.Compile(envelopeSchemaID)
	if err != nil {
		schemaInitErr = fmt.Errorf("crtx: compile envelope schema: %w", err)
		return
	}

	payloadSchemas = make(map[string]*jsonschema.Schema, len(payloadDefs))
	for topic, def := range payloadDefs {
		sch, err := c.Compile(eventsSchemaID + "#/$defs/" + def)
		if err != nil {
			schemaInitErr = fmt.Errorf("crtx: compile %s: %w", def, err)
			return
		}
		payloadSchemas[topic] = sch
	}
}

// ValidateEnvelopeBytes validates one JSON envelope document against
// the vendored crtx v0.1 envelope schema.
func ValidateEnvelopeBytes(doc []byte) error {
	schemaOnce.Do(compileSchemas)
	if schemaInitErr != nil {
		return schemaInitErr
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return fmt.Errorf("crtx: envelope not valid JSON: %w", err)
	}
	if err := envelopeSchema.Validate(inst); err != nil {
		return fmt.Errorf("crtx: envelope fails v0.1 schema: %w", err)
	}
	return nil
}

// ValidateEventPayload validates a canonical crtx event payload
// against the vendored events schema entry for topic. payload may be
// any JSON-marshalable value (typed struct or decoded map — both
// normalize through a JSON round-trip, matching the kit/bus payload
// type-erasure contract). Topics without a canonical payload schema
// (extensions, unknown names) are rejected.
func ValidateEventPayload(topic string, payload any) error {
	schemaOnce.Do(compileSchemas)
	if schemaInitErr != nil {
		return schemaInitErr
	}
	sch, ok := payloadSchemas[topic]
	if !ok {
		return fmt.Errorf("crtx: no canonical payload schema for topic %q", topic)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("crtx: marshal payload for %s: %w", topic, err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("crtx: payload not valid JSON: %w", err)
	}
	if err := sch.Validate(inst); err != nil {
		return fmt.Errorf("crtx: payload fails %s schema: %w", topic, err)
	}
	return nil
}
