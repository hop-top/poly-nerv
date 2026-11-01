package opencode

import (
	"fmt"
	"regexp"
	"strconv"
)

// pluginAPI is the OpenCode plugin API generation a release speaks.
type pluginAPI int

const (
	// apiV1 is the default-exported plugin function with string-keyed
	// hooks (OpenCode 1.x).
	apiV1 pluginAPI = 1
	// apiV2 is the Plugin.define({id, setup(ctx)}) model (OpenCode 2.x).
	apiV2 pluginAPI = 2
)

// versionRE finds the first MAJOR.MINOR.PATCH in `opencode --version`
// output: v1 prints "1.19.3", v2 prints "opencode v2.2.5".
var versionRE = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// apiForVersion maps an `opencode --version` string to its plugin API.
// Any 1.x (and earlier) release speaks v1, any 2.x release v2; an
// unparseable string or a major release nerv has no bridge for is an
// error, never a guess.
func apiForVersion(version string) (pluginAPI, error) {
	m := versionRE.FindStringSubmatch(version)
	if m == nil {
		return 0, fmt.Errorf(
			"opencode: cannot determine plugin API from version %q", version)
	}
	major, _ := strconv.Atoi(m[1]) // digits guaranteed by the regexp
	switch {
	case major <= 1:
		return apiV1, nil
	case major == 2:
		return apiV2, nil
	default:
		return 0, fmt.Errorf(
			"opencode: version %s has no nerv bridge (supported: 1.x, 2.x)", m[0])
	}
}
