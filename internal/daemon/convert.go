package daemon

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	nervv1 "hop.top/nerv/gen/go/nerv/v1"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/events"
	"hop.top/nerv/internal/merger"
)

// protoToEnvelope converts a wire proto Envelope into the internal
// envelope.Envelope used by the dispatch pipeline.
//
// Payload (*structpb.Struct) round-trips through JSON so the
// dispatcher and built-ins continue to consume json.RawMessage as
// they do in stdin mode. Returns an error only if the payload struct
// fails to marshal — which should not happen in practice for
// well-formed proto inputs.
//
// Nil input yields a non-nil zero Envelope rather than an error so
// the caller can hand it straight to the dispatcher without an extra
// nil-check (matches stdin-decode behaviour).
func protoToEnvelope(p *nervv1.Envelope) (*envelope.Envelope, error) {
	if p == nil {
		return &envelope.Envelope{}, nil
	}

	out := &envelope.Envelope{
		NervVersion: p.GetNervVersion(),
		Event:       p.GetEvent(),
		EventID:     p.GetEventId(),
		Synthesized: p.GetSynthesized(),
	}

	if ts := p.GetTimestamp(); ts != nil {
		out.Timestamp = ts.AsTime()
	}

	if cli := p.GetCli(); cli != nil {
		out.CLI = envelope.CLIInfo{
			Name:       cli.GetName(),
			Version:    cli.GetVersion(),
			BinaryPath: cli.GetBinaryPath(),
		}
	}

	if sess := p.GetSession(); sess != nil {
		out.Session = envelope.SessionInfo{
			ID:         sess.GetId(),
			ProjectKey: sess.GetProjectKey(),
			CWD:        sess.GetCwd(),
		}
	}

	if caps := p.GetCapabilities(); caps != nil {
		out.Capabilities = &envelope.CapabilityRef{
			CLI:             caps.GetCli(),
			CLIVersionRange: caps.GetCliVersionRange(),
		}
	}

	if payload := p.GetPayload(); payload != nil {
		raw, err := json.Marshal(payload.AsMap())
		if err != nil {
			return nil, fmt.Errorf("daemon: marshal payload: %w", err)
		}
		out.Payload = raw
	}

	if src := p.GetSynthesisSource(); src != nervv1.SynthesisSource_SYNTHESIS_SOURCE_UNSPECIFIED {
		name := src.String()
		out.SynthesisSource = &name
	}

	return out, nil
}

// decisionToProto converts an internal merger.Decision into the wire
// proto Decision. Rewrite + Metadata round-trip through JSON →
// structpb so any json-marshalable value the merger emits survives
// the conversion.
func decisionToProto(d merger.Decision) (*nervv1.Decision, error) {
	out := &nervv1.Decision{
		Action:  actionToProto(d.Action),
		Message: d.Message,
	}

	if d.Rewrite != nil {
		s, err := anyToStruct(d.Rewrite)
		if err != nil {
			return nil, fmt.Errorf("daemon: marshal rewrite: %w", err)
		}
		out.Rewrite = s
	}

	if len(d.Metadata) > 0 {
		s, err := structpb.NewStruct(d.Metadata)
		if err != nil {
			return nil, fmt.Errorf("daemon: marshal metadata: %w", err)
		}
		out.Metadata = s
	}

	return out, nil
}

// actionToProto maps a merger.Action onto the proto Action enum.
// Unknown actions default to ACTION_UNSPECIFIED to surface bugs
// loudly rather than silently allow.
func actionToProto(a merger.Action) nervv1.Action {
	switch a {
	case merger.Allow:
		return nervv1.Action_ACTION_ALLOW
	case merger.Warn:
		return nervv1.Action_ACTION_WARN
	case merger.Block:
		return nervv1.Action_ACTION_BLOCK
	case merger.Rewrite:
		return nervv1.Action_ACTION_REWRITE
	default:
		return nervv1.Action_ACTION_UNSPECIFIED
	}
}

// anyToStruct converts an arbitrary json-marshalable value into a
// google.protobuf.Struct. Top-level non-objects (arrays, scalars) are
// wrapped under a "value" key because Struct can only represent
// objects.
func anyToStruct(v any) (*structpb.Struct, error) {
	switch tv := v.(type) {
	case map[string]any:
		return structpb.NewStruct(tv)
	case nil:
		return nil, nil
	}

	// Fall back to JSON round-trip for typed maps or other shapes.
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err == nil {
		return structpb.NewStruct(m)
	}
	// Non-object — wrap under "value" so it survives the round trip.
	return structpb.NewStruct(map[string]any{"value": v})
}

// eventToProto wraps a known internal Event into the wire proto
// EventDescriptor used by EventService.List.
func eventToProto(e events.Event) *nervv1.EventDescriptor {
	return &nervv1.EventDescriptor{
		Name:     string(e),
		Category: events.Category(e),
		Origin:   events.Origin(e),
		Blocking: events.IsBlocking(e),
	}
}

// (Reserved for future use — silence unused-import warnings if the
// timestamppb import becomes optional.)
var _ = timestamppb.Now
