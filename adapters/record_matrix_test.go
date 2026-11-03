package adapters_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"hop.top/nerv/adapters"
)

const (
	recordWorkflow = "../.github/workflows/conformance-record.yaml"
	versionsFile   = "../versions.yaml"
	installScript  = "../scripts/install-cli.sh"
	dockerfileTest = "../docker/Dockerfile.test"
)

// installMethods are the methods scripts/install-cli.sh implements.
var installMethods = []string{"npm", "go", "pip", "github-release"}

var sha256RE = regexp.MustCompile(`^[0-9a-f]{64}$`)

type versionsEntry struct {
	Package  string   `yaml:"package"`
	Install  string   `yaml:"install"`
	Adapter  string   `yaml:"adapter"`
	Versions []string `yaml:"versions"`
	Release  struct {
		Asset  string                       `yaml:"asset"`
		Arch   map[string]string            `yaml:"arch"`
		Member string                       `yaml:"member"`
		Binary string                       `yaml:"binary"`
		SHA256 map[string]map[string]string `yaml:"sha256"`
	} `yaml:"release"`
}

func (e versionsEntry) adapterName(key string) string {
	if e.Adapter != "" {
		return e.Adapter
	}
	return key
}

func loadVersions(t *testing.T) map[string]versionsEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(versionsFile))
	if err != nil {
		t.Fatalf("read versions.yaml: %v", err)
	}
	var v struct {
		Matrix map[string]versionsEntry `yaml:"matrix"`
	}
	if err := yaml.Unmarshal(data, &v); err != nil {
		t.Fatalf("parse versions.yaml: %v", err)
	}
	return v.Matrix
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash(p))
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// TestVersionsCoverRegistry: every registered adapter has at least one
// versions.yaml entry, and every entry maps to a registered adapter.
// versions.yaml drives the record matrix.
func TestVersionsCoverRegistry(t *testing.T) {
	have := map[string]bool{}
	for key, e := range loadVersions(t) {
		name := e.adapterName(key)
		if _, ok := adapters.Registry()[name]; !ok {
			t.Errorf("versions.yaml entry %q maps to adapter %q: "+
				"not registered (set `adapter:`)", key, name)
		}
		have[name] = true
		if len(e.Versions) == 0 {
			t.Errorf("versions.yaml entry %q has no versions", key)
		}
	}
	var missing []string
	for name := range adapters.Registry() {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("adapter %q registered but has no versions.yaml "+
			"entry; the record matrix would skip it", name)
	}
}

// TestVersionsInstallMethodsSupported: every install method in
// versions.yaml is implemented by the shared install script.
func TestVersionsInstallMethodsSupported(t *testing.T) {
	script := readFile(t, installScript)
	known := map[string]bool{}
	for _, m := range installMethods {
		known[m] = true
		if !strings.Contains(script, m+")") {
			t.Errorf("install script has no %q branch", m)
		}
	}
	for key, e := range loadVersions(t) {
		if !known[e.Install] {
			t.Errorf("%s: install method %q unsupported (want %v)",
				key, e.Install, installMethods)
		}
	}
}

// TestGithubReleaseChecksums: github-release entries pin a sha256 per
// arch for every listed version; nothing is downloaded unverified.
func TestGithubReleaseChecksums(t *testing.T) {
	for key, e := range loadVersions(t) {
		if e.Install != "github-release" {
			continue
		}
		r := e.Release
		if r.Asset == "" || r.Member == "" || r.Binary == "" ||
			len(r.Arch) == 0 {
			t.Errorf("%s: release needs asset, arch, member, binary", key)
		}
		if !strings.Contains(r.Asset, "{arch}") {
			t.Errorf("%s: release.asset lacks {arch}", key)
		}
		for _, ver := range e.Versions {
			for arch := range r.Arch {
				sum := r.SHA256[ver][arch]
				if !sha256RE.MatchString(sum) {
					t.Errorf("%s %s/%s: missing or malformed "+
						"release.sha256 (%q)", key, ver, arch, sum)
				}
			}
		}
	}
}

// TestRecordWorkflowMatrixFromVersions: the record matrix is generated
// from versions.yaml (no hand-kept list), installs through the shared
// script, targets one adapter per job, and stays least-privilege.
func TestRecordWorkflowMatrixFromVersions(t *testing.T) {
	var wf struct {
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			Needs       any               `yaml:"needs"`
			Permissions map[string]string `yaml:"permissions"`
			Outputs     map[string]string `yaml:"outputs"`
			Strategy    struct {
				Matrix string `yaml:"matrix"`
			} `yaml:"strategy"`
			Steps []struct {
				Env map[string]string `yaml:"env"`
				Run string            `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(
		[]byte(readFile(t, recordWorkflow)), &wf); err != nil {
		t.Fatalf("parse workflow: %v", err)
	}

	plan, ok := wf.Jobs["plan"]
	if !ok {
		t.Fatal("record workflow has no plan job generating the matrix")
	}
	if plan.Outputs["matrix"] == "" {
		t.Error("plan job has no matrix output")
	}
	fromVersions := false
	for _, s := range plan.Steps {
		if strings.Contains(s.Run, "versions.yaml") {
			fromVersions = true
		}
	}
	if !fromVersions {
		t.Error("plan job does not read versions.yaml")
	}

	rec, ok := wf.Jobs["record"]
	if !ok {
		t.Fatal("record workflow has no record job")
	}
	if !strings.Contains(rec.Strategy.Matrix,
		"fromJSON(needs.plan.outputs.matrix)") {
		t.Errorf("record matrix = %q; want fromJSON of plan output",
			rec.Strategy.Matrix)
	}
	installs, filtered := false, false
	for _, s := range rec.Steps {
		if strings.Contains(s.Run, "scripts/install-cli.sh") &&
			strings.Contains(s.Run, "matrix.key") {
			installs = true
		}
		if s.Env[cliFilterEnv] == "${{ matrix.adapter }}" &&
			strings.Contains(s.Run, "TestConformance") {
			filtered = true
		}
	}
	if !installs {
		t.Error("record job does not install via " +
			"scripts/install-cli.sh ${{ matrix.key }}")
	}
	if !filtered {
		t.Errorf("record job does not pass ${{ matrix.adapter }} via %s",
			cliFilterEnv)
	}

	// Least privilege: workflow default empty; plan reads; record
	// writes contents (cassette commit) and nothing else.
	if wf.Permissions == nil || len(wf.Permissions) != 0 {
		t.Errorf("top-level permissions = %v; want {}", wf.Permissions)
	}
	if got := plan.Permissions; len(got) != 1 || got["contents"] != "read" {
		t.Errorf("plan permissions = %v; want contents: read", got)
	}
	if got := rec.Permissions; len(got) != 1 ||
		got["contents"] != "write" {
		t.Errorf("record permissions = %v; want contents: write only",
			got)
	}
}

// TestDockerfileUsesSharedInstall: CLI release sums live only in
// versions.yaml. Base-image digest pins (`image@sha256:...`) are image
// provenance, not CLI sums, and are allowed.
func TestDockerfileUsesSharedInstall(t *testing.T) {
	df := readFile(t, dockerfileTest)
	if !strings.Contains(df, "scripts/install-cli.sh") {
		t.Error("Dockerfile.test must install via scripts/install-cli.sh")
	}
	df = regexp.MustCompile(`@sha256:[0-9a-f]{64}`).ReplaceAllString(df, "@sha256:<digest>")
	if m := regexp.MustCompile(`[0-9a-f]{64}`).FindString(df); m != "" {
		t.Errorf("Dockerfile.test hardcodes a sha256 (%s...); "+
			"keep sums in versions.yaml", m[:12])
	}
}
