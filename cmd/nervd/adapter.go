package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"hop.top/nerv/adapters/aider"
	"hop.top/nerv/adapters/claude"
	"hop.top/nerv/adapters/codex"
	"hop.top/nerv/adapters/gemini"
	"hop.top/nerv/adapters/opencode"
	"hop.top/nerv/adapters/openhands"
	"hop.top/nerv/adapters/vibe"
	"hop.top/nerv/internal/envelope"
	"hop.top/nerv/internal/merger"
)

// adapterFlag selects the host CLI whose native dialect stdin/stdout
// use in stateless mode. Hook commands written by `nerv install` carry
// it (e.g. `nervd --adapter vibe`). Unset keeps the legacy behavior:
// hook_event_name input is read as Claude Code, anything else as a
// nerv envelope.
var adapterFlag = flag.String("adapter", "",
	"host CLI dialect for stateless stdin/stdout (aider, codex, gemini, opencode, openhands, vibe)")

// adapterRoute translates one host CLI's native hook I/O.
type adapterRoute struct {
	// decode turns native stdin into an envelope plus the native event
	// name (the name the host uses, which keys context channels).
	decode func(raw []byte) (*envelope.Envelope, string, error)
	// encode renders a decision as the host's stdout bytes and exit code.
	encode func(d merger.Decision, event string) ([]byte, int, error)
	// fromArgs, when set, replaces stdin: the host passes argv and no
	// JSON, and may share its terminal's stdin, which must stay unread.
	// It receives the positional args after the flags.
	fromArgs func(args []string) ([]byte, error)
}

// adapterRoutes is the stateless-mode translator table. Keys match the
// canonical CLI names in adapters.Registry.
var adapterRoutes = map[string]adapterRoute{
	"aider":     {decodeAider, encodeAider, aiderArgs},
	"codex":     {decodeCodex, encodeCodex, nil},
	"gemini":    {decodeGemini, encodeGemini, nil},
	"openhands": {decodeOpenHands, encodeOpenHands, nil},
	"vibe":      {decodeVibe, encodeVibe, nil},
	"opencode":  {decodeOpenCode, encodeOpenCode, nil},
}

// supportedAdapters lists the valid --adapter values, sorted.
func supportedAdapters() []string {
	names := make([]string, 0, len(adapterRoutes))
	for n := range adapterRoutes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// lookupAdapter resolves an --adapter value. "" yields (nil, nil):
// legacy routing. "claude" is accepted and also yields legacy routing,
// which is the claude dialect already.
func lookupAdapter(name string) (*adapterRoute, error) {
	if name == "" || name == "claude" {
		return nil, nil
	}
	r, ok := adapterRoutes[name]
	if !ok {
		return nil, fmt.Errorf("unknown adapter %q (supported: claude, %s)",
			name, strings.Join(supportedAdapters(), ", "))
	}
	return &r, nil
}

func probeMap(raw []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return m, nil
}

func strField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, _ := m[k].(string); s != "" {
			return s
		}
	}
	return ""
}

// aiderArgs turns `nervd --adapter aider lint <file>...` / `test` argv
// into the synthesized event JSON. Aider runs lint-cmd/test-cmd with
// the process cwd as the repo root.
func aiderArgs(args []string) ([]byte, error) {
	cwd, _ := os.Getwd()
	return aider.BridgeEvent(args, cwd)
}

func decodeAider(raw []byte) (*envelope.Envelope, string, error) {
	m, err := probeMap(raw)
	if err != nil {
		return nil, "", err
	}
	env, err := aider.ToEnvelope(m)
	return env, strField(m, "hook_event_name"), err
}

func encodeAider(d merger.Decision, ev string) ([]byte, int, error) {
	return aider.FromEventDecision(d, ev)
}

func decodeCodex(raw []byte) (*envelope.Envelope, string, error) {
	m, err := probeMap(raw)
	if err != nil {
		return nil, "", err
	}
	env, err := (&claude.Translator{}).ToEnvelope(m)
	return env, strField(m, "hook_event_name"), err
}

func encodeCodex(d merger.Decision, ev string) ([]byte, int, error) {
	return codex.FromEventDecision(d, ev)
}

func decodeGemini(raw []byte) (*envelope.Envelope, string, error) {
	m, err := probeMap(raw)
	if err != nil {
		return nil, "", err
	}
	env, err := gemini.ToEnvelope(m)
	return env, strField(m, "hook_event_name"), err
}

func encodeGemini(d merger.Decision, ev string) ([]byte, int, error) {
	return marshalMap(gemini.FromEventDecision(d, ev))
}

func decodeOpenHands(raw []byte) (*envelope.Envelope, string, error) {
	m, err := probeMap(raw)
	if err != nil {
		return nil, "", err
	}
	env, err := openhands.ToEnvelope(m)
	return env, strField(m, "hook_event_name", "event"), err
}

func encodeOpenHands(d merger.Decision, ev string) ([]byte, int, error) {
	return marshalMap(openhands.FromEventDecision(d, ev))
}

func decodeVibe(raw []byte) (*envelope.Envelope, string, error) {
	m, err := probeMap(raw)
	if err != nil {
		return nil, "", err
	}
	env, err := vibe.ToEnvelope(m)
	return env, strField(m, "event"), err
}

func encodeVibe(d merger.Decision, ev string) ([]byte, int, error) {
	return marshalMap(vibe.FromEventDecision(d, ev))
}

func decodeOpenCode(raw []byte) (*envelope.Envelope, string, error) {
	m, err := probeMap(raw)
	if err != nil {
		return nil, "", err
	}
	env, err := opencode.ToEnvelope(raw, "")
	return env, strField(m, "type"), err
}

func encodeOpenCode(d merger.Decision, ev string) ([]byte, int, error) {
	b, err := opencode.FromEventDecision(d, ev)
	return b, 0, err
}

// marshalMap adapts the (map, code, err) translator shape to bytes.
func marshalMap(m map[string]any, code int, err error) ([]byte, int, error) {
	if err != nil {
		return nil, 0, err
	}
	b, err := json.Marshal(m)
	return b, code, err
}

// respondAdapter writes the encoded decision to out (newline-terminated)
// and returns the exit code. Warn text goes to errw as on the claude
// path. Encode failures fail open (exit 0, nothing written).
func respondAdapter(out, errw io.Writer, r *adapterRoute, event string, d merger.Decision) int {
	if d.Action == merger.Warn && d.Message != "" {
		_, _ = fmt.Fprintf(errw, "nervd: warn: %s\n", d.Message)
	}
	b, code, err := r.encode(d, event)
	if err != nil {
		_, _ = fmt.Fprintf(errw, "nervd: respond: %v\n", err)
		return 0
	}
	if len(b) > 0 {
		_, _ = out.Write(append(b, '\n'))
	}
	return code
}
