module hop.top/nerv/sdk/go

go 1.27.3

require (
	connectrpc.com/connect v1.21.1
	google.golang.org/protobuf v1.36.12
	hop.top/nerv v0.1.0-alpha.0 // x-release-please-version
)

// Local development only. Consumers never see this replace (Go ignores
// replace directives outside the main module), so they resolve the
// require above against the published hop.top/nerv tag. The go.work at
// the repo root already stitches both modules; this replace keeps
// `GOWORK=off` builds and `go mod tidy` in this directory working
// before that tag exists. release-please rewrites the version on the
// require line (x-release-please-version) so it always names the
// root release cut alongside this module (linked-versions group).
replace hop.top/nerv => ../..
