// Package envelope implements streaming encode/decode for nerv envelopes
// with configurable size limits. Envelopes exceeding the limit produce
// an error; the caller decides fail-open vs fail-safe policy.
package envelope

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

// DefaultMaxEnvelopeSize is the soft ceiling (1 MB).
const DefaultMaxEnvelopeSize int64 = 1 << 20

// HardMaxEnvelopeSize is the absolute ceiling (10 MB).
// Even NERV_MAX_ENVELOPE_SIZE cannot exceed this.
const HardMaxEnvelopeSize int64 = 10 << 20

// CLIInfo identifies the CLI that produced the event.
type CLIInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	BinaryPath string `json:"binary_path,omitempty"`
}

// SessionInfo provides session context for an event.
type SessionInfo struct {
	ID         string `json:"id"`
	ProjectKey string `json:"project_key,omitempty"`
	CWD        string `json:"cwd,omitempty"`
}

// CapabilityRef references the adapter's capability map.
type CapabilityRef struct {
	CLI             string `json:"cli"`
	CLIVersionRange string `json:"cli_version_range"`
}

// Envelope is the wire format for all nerv hook events.
type Envelope struct {
	NervVersion     string          `json:"nerv_version"`
	Event           string          `json:"event"`
	EventID         string          `json:"event_id"`
	Timestamp       time.Time       `json:"timestamp"`
	CLI             CLIInfo         `json:"cli"`
	Session         SessionInfo     `json:"session"`
	Payload         json.RawMessage `json:"payload"`
	Synthesized     bool            `json:"synthesized,omitempty"`
	SynthesisSource *string         `json:"synthesis_source,omitempty"`
	Capabilities    *CapabilityRef  `json:"capabilities,omitempty"`
}

// Encode streams env as JSON into w without buffering the full output.
func Encode(w io.Writer, env *Envelope) error {
	return json.NewEncoder(w).Encode(env)
}

// Decode reads a JSON envelope from r, enforcing the configured size
// limit. Returns an error if the payload exceeds the limit or is
// invalid JSON.
func Decode(r io.Reader) (*Envelope, error) {
	limit := resolveMaxSize()
	lr := &io.LimitedReader{R: r, N: limit + 1} // +1 to detect overflow

	var env Envelope
	dec := json.NewDecoder(lr)
	if err := dec.Decode(&env); err != nil {
		return nil, fmt.Errorf("envelope: decode: %w", err)
	}

	// The decoder may stop right after the value, leaving trailing bytes
	// (the encoder's newline) unread. Count them toward the limit: the
	// input exceeded it only if all limit+1 bytes were consumed.
	if _, err := io.Copy(io.Discard, lr); err != nil {
		return nil, fmt.Errorf("envelope: read: %w", err)
	}
	if lr.N == 0 {
		return nil, fmt.Errorf(
			"envelope: size exceeds limit (%d bytes)", limit,
		)
	}

	return &env, nil
}

// resolveMaxSize returns the effective max envelope size, respecting
// NERV_MAX_ENVELOPE_SIZE but never exceeding HardMaxEnvelopeSize.
func resolveMaxSize() int64 {
	raw := os.Getenv("NERV_MAX_ENVELOPE_SIZE")
	if raw == "" {
		return DefaultMaxEnvelopeSize
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return DefaultMaxEnvelopeSize
	}
	if v > HardMaxEnvelopeSize {
		return HardMaxEnvelopeSize
	}
	return v
}
