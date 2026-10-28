package nervsdk

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The published go.mod must require a real hop.top/nerv release: the
// local replace is ignored by consumers, so a placeholder pseudo-version
// would make `go get hop.top/nerv/sdk/go` fail. The release marker lets
// release-please keep the version in lockstep with the root module.
func TestGoMod_RequiresReleasedNerv(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?m)^\s*hop\.top/nerv (v\S+)(.*)$`)
	m := re.FindStringSubmatch(string(data))
	if m == nil {
		t.Fatal("go.mod has no require for hop.top/nerv")
	}
	version, rest := m[1], m[2]
	if !regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`).MatchString(version) ||
		strings.HasPrefix(version, "v0.0.0-") {
		t.Errorf("hop.top/nerv require = %s, want a tagged release version", version)
	}
	if !strings.Contains(rest, "x-release-please-version") {
		t.Errorf("hop.top/nerv require line lacks x-release-please-version marker: %q", m[0])
	}
}
