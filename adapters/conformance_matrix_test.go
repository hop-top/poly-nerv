package adapters_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"hop.top/nerv/adapters"
)

// replayWorkflow is the CI workflow whose matrix must cover every
// registered adapter.
const replayWorkflow = "../.github/workflows/conformance-replay.yaml"

// cliFilterEnv selects a single adapter for the conformance suite.
// Unset: all adapters run. Set: must name a known adapter.
const cliFilterEnv = "NERV_CONFORMANCE_CLI"

// resolveCLIFilter validates the filter value. set=false means no
// filter; set=true with an empty or unknown value is an error.
func resolveCLIFilter(
	val string, set bool, known []string,
) (string, error) {
	if !set {
		return "", nil
	}
	if val == "" {
		return "", errFilter("empty " + cliFilterEnv +
			"; known: " + strings.Join(known, ", "))
	}
	for _, k := range known {
		if k == val {
			return val, nil
		}
	}
	return "", errFilter("unknown " + cliFilterEnv + "=" + val +
		"; known: " + strings.Join(known, ", "))
}

type errFilter string

func (e errFilter) Error() string { return string(e) }

func TestResolveCLIFilter(t *testing.T) {
	known := []string{"claude", "codex"}
	cases := []struct {
		name    string
		val     string
		set     bool
		want    string
		wantErr bool
	}{
		{"unset runs all", "", false, "", false},
		{"known", "codex", true, "codex", false},
		{"empty fails", "", true, "", true},
		{"unknown fails", "vibe", true, "", true},
		{"case sensitive", "Claude", true, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveCLIFilter(c.val, c.set, known)
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// TestReplayWorkflowMatrixCoversRegistry flags a registered adapter
// missing from the workflow matrix, or a matrix entry with no adapter.
func TestReplayWorkflowMatrixCoversRegistry(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash(replayWorkflow))
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	var wf struct {
		Jobs map[string]struct {
			Strategy struct {
				Matrix struct {
					CLI []string `yaml:"cli"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
			Steps []struct {
				Env map[string]string `yaml:"env"`
				Run string            `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatalf("parse workflow: %v", err)
	}
	job, ok := wf.Jobs["replay"]
	if !ok {
		t.Fatal("workflow has no replay job")
	}

	have := map[string]bool{}
	for _, c := range job.Strategy.Matrix.CLI {
		have[c] = true
	}
	var want []string
	for name := range adapters.Registry() {
		want = append(want, name)
	}
	sort.Strings(want)
	for _, name := range want {
		if !have[name] {
			t.Errorf("adapter %q registered but missing from "+
				"replay matrix; add it to matrix.cli", name)
		}
		delete(have, name)
	}
	for extra := range have {
		t.Errorf("matrix.cli entry %q has no registered adapter", extra)
	}

	// matrix.cli must actually reach the test filter.
	wired := false
	for _, s := range job.Steps {
		if s.Env[cliFilterEnv] == "${{ matrix.cli }}" &&
			strings.Contains(s.Run, "TestConformance") {
			wired = true
		}
	}
	if !wired {
		t.Errorf("no step passes ${{ matrix.cli }} via %s to the "+
			"conformance test run", cliFilterEnv)
	}
}
