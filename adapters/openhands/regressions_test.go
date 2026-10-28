package openhands

import (
	"encoding/json"
	"strings"
	"testing"

	"hop.top/nerv/internal/events"
)

// --- Issue 7: StopFailure synthesis from ErrorObservation ---

func TestRegression_ErrorObservation_AgentError_ProducesStopFailure(
	t *testing.T,
) {
	// ErrorObservation with source=agent and no tool context should
	// produce StopFailure, not PostToolUseFailure.
	payload := map[string]any{
		"observation": "ErrorObservation",
		"error":       "agent crashed",
		"source":      "agent",
	}
	synth := SynthesizeFromEventStream(payload)
	if !hasSynthEvent(synth, events.EventStopFailure) {
		t.Error(
			"ErrorObservation with source=agent should produce " +
				"StopFailure",
		)
	}
	if hasSynthEvent(synth, events.EventPostToolUseFailure) {
		t.Error(
			"ErrorObservation with source=agent should NOT " +
				"produce PostToolUseFailure",
		)
	}
}

func TestRegression_ErrorObservation_ToolError_ProducesPostToolUseFailure(
	t *testing.T,
) {
	// ErrorObservation with a tool_name should still produce
	// PostToolUseFailure.
	payload := map[string]any{
		"observation": "ErrorObservation",
		"error":       "non-zero exit",
		"tool_name":   "Bash",
	}
	synth := SynthesizeFromEventStream(payload)
	if !hasSynthEvent(synth, events.EventPostToolUseFailure) {
		t.Error(
			"ErrorObservation with tool_name should produce " +
				"PostToolUseFailure",
		)
	}
	if hasSynthEvent(synth, events.EventStopFailure) {
		t.Error(
			"ErrorObservation with tool_name should NOT produce " +
				"StopFailure",
		)
	}
}

func TestRegression_ErrorObservation_FinishAction_ProducesStopFailure(
	t *testing.T,
) {
	// ErrorObservation with action_type=FINISH should produce
	// StopFailure.
	payload := map[string]any{
		"observation": "ErrorObservation",
		"error":       "finish failed",
		"action_type": "FINISH",
	}
	synth := SynthesizeFromEventStream(payload)
	if !hasSynthEvent(synth, events.EventStopFailure) {
		t.Error(
			"ErrorObservation with action_type=FINISH should " +
				"produce StopFailure",
		)
	}
}

// --- Issue 8: MergeConfig JSON preservation ---

func TestRegression_MergeConfig_PreservesExistingJSON(t *testing.T) {
	a := &Adapter{}
	existing := []byte(`{"existing_key": "preserved"}`)
	merged, err := a.MergeConfig(existing)
	if err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}
	if !strings.Contains(string(merged), "existing_key") {
		t.Error(
			"MergeConfig did not preserve existing JSON key " +
				"existing_key",
		)
	}

	// Also verify the value is intact.
	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatalf("merged output not valid JSON: %v", err)
	}
	if m["existing_key"] != "preserved" {
		t.Errorf(
			"existing_key = %v, want \"preserved\"",
			m["existing_key"],
		)
	}
}
