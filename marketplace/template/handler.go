// nerv Hook Handler Template (Go)
//
// Build: go build -o handler handler.go
// Reads the JSON envelope from stdin, writes a JSON decision to stdout.
//
// Exit codes (spec/handler-contract.md); the stdout "action" must match:
//
//	0 — allow   (event proceeds unchanged)
//	1 — warn    ("message" shown to the user)
//	2 — block   (event rejected; "message" shown to the user)
//	3 — rewrite (event proceeds with the "rewrite" payload)
//	>=4 — error (fail-open: logged, event proceeds)
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Envelope holds the fields this handler reads; event-specific fields
// live under payload (spec/events.yaml).
type Envelope struct {
	Event   string `json:"event"`
	Payload struct {
		ToolName  string         `json:"tool_name"`
		ToolInput map[string]any `json:"tool_input"`
	} `json:"payload"`
}

// Decision is the handler's stdout reply.
type Decision struct {
	Action  string `json:"action"`
	Message string `json:"message,omitempty"`
}

func main() {
	var env Envelope
	if err := json.NewDecoder(os.Stdin).Decode(&env); err != nil {
		fmt.Fprintf(os.Stderr, "hook: decode error: %v\n", err)
		os.Exit(4) // error: fail-open
	}

	// --- Your logic here ---
	// To block: emit Decision{Action: "block", Message: "why"} and exit 2.

	// Example: allow everything.
	_ = json.NewEncoder(os.Stdout).Encode(Decision{Action: "allow"})
}
