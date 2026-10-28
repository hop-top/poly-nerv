package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// capabilitiesJSON mirrors the introspection document shape the
// conformance contract expects: a leaves array where every entry
// carries a full command path and leaf name.
type capabilitiesJSON struct {
	Tool          string `json:"tool"`
	SchemaVersion string `json:"schema_version"`
	Leaves        []struct {
		Path  string `json:"path"`
		Name  string `json:"name"`
		Flags []struct {
			Name string `json:"name"`
		} `json:"flags"`
	} `json:"leaves"`
}

// TestCapabilities_MachineReadableSurface locks in factor-5: the
// binary must self-describe its command surface as JSON — previously
// `nerv capabilities` fell through to human help text.
func TestCapabilities_MachineReadableSurface(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()

	stdout, stderr, code := runInProject(t, bin, project,
		"capabilities", "--format", "json")
	if code != 0 {
		t.Fatalf("capabilities exited %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}

	var doc capabilitiesJSON
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not capabilities JSON: %v\nstdout:\n%s", err, stdout)
	}
	if doc.Tool != "nerv" {
		t.Errorf("tool = %q, want nerv", doc.Tool)
	}
	if len(doc.Leaves) == 0 {
		t.Fatal("capabilities document has no leaves")
	}

	byPath := map[string]int{}
	for i, l := range doc.Leaves {
		if l.Path == "" || l.Name == "" {
			t.Errorf("leaf %d has empty path/name: %+v", i, l)
		}
		byPath[l.Path] = i
	}

	// Representative depth-1 leaf with its local flag surface.
	addIdx, ok := byPath["nerv add"]
	if !ok {
		t.Fatalf("leaves missing \"nerv add\"; got %v", pathsOf(doc))
	}
	var hasSource bool
	for _, f := range doc.Leaves[addIdx].Flags {
		if f.Name == "source" {
			hasSource = true
		}
	}
	if !hasSource {
		t.Errorf("nerv add leaf does not declare --source flag: %+v", doc.Leaves[addIdx])
	}

	// Representative nested leaf.
	if _, ok := byPath["nerv daemon status"]; !ok {
		t.Errorf("leaves missing \"nerv daemon status\"; got %v", pathsOf(doc))
	}
}

func pathsOf(doc capabilitiesJSON) []string {
	out := make([]string, 0, len(doc.Leaves))
	for _, l := range doc.Leaves {
		out = append(out, l.Path)
	}
	return out
}

// TestCapabilities_TableModeForHumans keeps the human path working:
// bare `nerv capabilities` renders a table, not JSON, and exits 0.
func TestCapabilities_TableModeForHumans(t *testing.T) {
	bin := buildNerv(t)
	project := t.TempDir()

	stdout, _, code := runInProject(t, bin, project, "capabilities")
	if code != 0 {
		t.Fatalf("capabilities exited %d", code)
	}
	if !strings.Contains(stdout, "nerv add") {
		t.Errorf("table output does not list leaves:\n%s", stdout)
	}
}
