package opencode

import (
	"encoding/json"
	"strings"
	"testing"

	"hop.top/nerv/internal/merger"
)

func decodeDJ(t *testing.T, b []byte) DecisionJSON {
	t.Helper()
	var dj DecisionJSON
	if err := json.Unmarshal(b, &dj); err != nil {
		t.Fatalf("unmarshal %q: %v", b, err)
	}
	return dj
}

func TestFromEventDecision_Context_SupportedEvents(t *testing.T) {
	for _, ev := range []string{OCToolExecAfter, OCSessionCreated} {
		for _, a := range []merger.Action{merger.Allow, merger.Warn, merger.Rewrite} {
			t.Run(ev+"/"+string(a), func(t *testing.T) {
				b, err := FromEventDecision(
					merger.Decision{Action: a, Context: "ctx"}, ev)
				if err != nil {
					t.Fatal(err)
				}
				if got := decodeDJ(t, b).Context; got != "ctx" {
					t.Fatalf("context = %q, want ctx (%s)", got, b)
				}
			})
		}
	}
}

func TestFromEventDecision_Context_NoChannelEventsDropped(t *testing.T) {
	for _, ev := range []string{
		OCToolExecBefore, OCSessionIdle, OCSessionDeleted, OCSessionError,
		OCSessionCompacted, OCFileEdited, OCPermissionAsked,
		OCPermissionReplied, OCTUIPromptAppend, OCTodoUpdated, "",
	} {
		t.Run(ev, func(t *testing.T) {
			b, err := FromEventDecision(
				merger.Decision{Action: merger.Allow, Context: "ctx"}, ev)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), "context") {
				t.Fatalf("%q has no context channel, got %s", ev, b)
			}
		})
	}
}

func TestFromEventDecision_Context_DroppedOnBlock(t *testing.T) {
	b, err := FromEventDecision(
		merger.Decision{Action: merger.Block, Message: "no", Context: "ctx"},
		OCToolExecAfter)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "context") {
		t.Fatalf("block must not carry context: %s", b)
	}
}

func TestFromDecision_DropsContext(t *testing.T) {
	b, err := FromDecision(merger.Decision{Action: merger.Allow, Context: "ctx"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "context") {
		t.Fatalf("FromDecision has no event, must drop context: %s", b)
	}
}

// The bridge applies context only through the two upstream channels:
// tool.execute.after output.output (appended to the tool result the
// model sees) and experimental.chat.system.transform output.system
// (session-start context).
func TestGeneratePlugin_ContextChannels(t *testing.T) {
	src, err := GeneratePlugin("/usr/local/bin/nervd")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`context?: string;`,
		`"experimental.chat.system.transform"`,
		`output.system.push(`,
		`output.output +=`,
		`sessionContext`,
	} {
		if !strings.Contains(src, s) {
			t.Errorf("plugin missing %q", s)
		}
	}
	// No other hook may write to the model-facing output.
	if n := strings.Count(src, "output.output +="); n != 1 {
		t.Errorf("output.output mutated %d times, want 1", n)
	}
}
