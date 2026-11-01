package main

import (
	"hop.top/nerv/adapters/goose"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// decodeGoose reads a Goose plugin-hook payload; the native event name is
// the "event" field.
func decodeGoose(raw []byte) (*envelope.Envelope, string, error) {
	m, err := probeMap(raw)
	if err != nil {
		return nil, "", err
	}
	env, err := goose.ToEnvelope(m)
	return env, strField(m, "event"), err
}

// encodeGoose answers in Goose's dialect: block rides stdout as
// {"decision":"block","reason":...} with exit 0 (Goose reads the decision
// from stdout regardless of exit status); every other outcome is empty
// stdout, exit 0.
func encodeGoose(d merger.Decision, ev string) ([]byte, int, error) {
	m, code, err := goose.FromEventDecision(d, ev)
	if err != nil || m == nil {
		return nil, code, err
	}
	return marshalMap(m, code, nil)
}
