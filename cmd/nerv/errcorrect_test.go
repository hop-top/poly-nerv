package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// errEnvelope mirrors the kit output.Error JSON shape rendered on
// stderr under --format json.
type errEnvelope struct {
	Code         string   `json:"code"`
	Message      string   `json:"message"`
	Cause        string   `json:"cause"`
	SuggestedFix string   `json:"suggested_fix"`
	Alternatives []string `json:"alternatives"`
	ExitCode     int      `json:"exit_code"`
}

// decodeErrEnvelope extracts the leading JSON envelope from stderr
// (fang appends its own styled error block after it).
func decodeErrEnvelope(t *testing.T, stderr string) errEnvelope {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stderr))
	var env errEnvelope
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("stderr does not start with a JSON error envelope: %v\nstderr:\n%s",
			err, stderr)
	}
	return env
}

// seedEventLog writes a JSONL event-log file with the given event ids
// into the sandbox's XDG state tree.
func seedEventLog(t *testing.T, project string, ids ...string) {
	t.Helper()
	dir := filepath.Join(project, "xdg-state", "nerv", "events")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir event log: %v", err)
	}
	var lines strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&lines,
			`{"envelope":{"nerv_version":"1.0","event":"tool_call","event_id":%q,"timestamp":%q,"cli":{"name":"claude","version":"1.0"},"session":{"id":"s1"},"payload":{}},"decision":{"action":"allow"}}`+"\n",
			id, time.Now().UTC().Format(time.RFC3339))
	}
	path := filepath.Join(dir, "events-"+time.Now().UTC().Format("2006-01-02")+".jsonl")
	if err := os.WriteFile(path, []byte(lines.String()), 0o640); err != nil {
		t.Fatalf("seed event log: %v", err)
	}
}

// TestReplayMissingEvent_EnvelopeOffersCorrection locks in factor-4:
// a not-found replay must return a structured envelope that carries a
// machine-actionable recovery path, not just code+message.
func TestReplayMissingEvent_EnvelopeOffersCorrection(t *testing.T) {
	bin := buildNerv(t)

	t.Run("empty_log_suggests_discovery", func(t *testing.T) {
		project := t.TempDir()
		_, stderr, code := runInProject(t, bin, project,
			"replay", "evt-does-not-exist", "--format", "json")
		if code != 3 {
			t.Fatalf("exit %d, want NOT_FOUND=3\nstderr:\n%s", code, stderr)
		}
		env := decodeErrEnvelope(t, stderr)
		if env.Code != "NOT_FOUND" || env.ExitCode != 3 {
			t.Errorf("envelope code/exit = %s/%d, want NOT_FOUND/3", env.Code, env.ExitCode)
		}
		if env.SuggestedFix == "" {
			t.Error("envelope has no suggested_fix")
		}
		if !strings.Contains(env.SuggestedFix, "nerv tail") {
			t.Errorf("suggested_fix does not point at event discovery:\n%s", env.SuggestedFix)
		}
	})

	t.Run("recorded_ids_offered_as_alternatives", func(t *testing.T) {
		project := t.TempDir()
		seedEventLog(t, project, "evt-aaa", "evt-bbb")
		_, stderr, code := runInProject(t, bin, project,
			"replay", "evt-abc", "--format", "json")
		if code != 3 {
			t.Fatalf("exit %d, want NOT_FOUND=3\nstderr:\n%s", code, stderr)
		}
		env := decodeErrEnvelope(t, stderr)
		if len(env.Alternatives) == 0 {
			t.Fatalf("envelope offers no alternatives\nstderr:\n%s", stderr)
		}
		joined := strings.Join(env.Alternatives, "\n")
		if !strings.Contains(joined, "evt-aaa") || !strings.Contains(joined, "evt-bbb") {
			t.Errorf("alternatives do not list recorded event ids:\n%s", joined)
		}
	})
}

// TestInboxShowMissing_EnvelopeOffersCorrection extends the same
// contract to the other user-facing not-found path: an unknown inbox
// message id must be NOT_FOUND-classed and point at `nerv inbox list`.
func TestInboxShowMissing_EnvelopeOffersCorrection(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()

	_, stderr, code := runInProject(t, bin, project,
		"inbox", "show", "m-missing", "--format", "json")
	if code != 3 {
		t.Fatalf("exit %d, want NOT_FOUND=3\nstderr:\n%s", code, stderr)
	}
	env := decodeErrEnvelope(t, stderr)
	if env.Code != "NOT_FOUND" {
		t.Errorf("envelope code = %s, want NOT_FOUND", env.Code)
	}
	if !strings.Contains(env.SuggestedFix, "nerv inbox list") {
		t.Errorf("suggested_fix does not point at `nerv inbox list`:\n%s", env.SuggestedFix)
	}
}
