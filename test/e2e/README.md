# test/e2e — End-to-End Tests

End-to-end tests that validate system-level behaviours across real
binaries, docs, and the full build graph. Not unit tests — they spawn
subprocesses and read the actual repo tree.

## Contributor Smoke Test

**File:** `contributor_smoketest_test.go`
**Covers:** US-0019 — OSS Contributor First PR Path

Validates the contributor experience: clone → test → read docs → add
a built-in — without environment drift or mystery errors.

### Run

```
# Fast: skip wall-clock and fs-mutation tests
go test ./test/e2e/... -run TestContributorSmoke -v -short

# Full: includes timing gate and destructive-defaults check
go test ./test/e2e/... -run TestContributorSmoke -v -timeout 5m

# Budget override (seconds) for slow CI runners
NERV_SMOKE_TIMEOUT=90 go test ./test/e2e/... -run TestContributorSmoke -v
```

### Tests

| Test | Passes | Skip condition |
|------|--------|----------------|
| `TestContributorSmoke_MakeTestUnder60s` | Warns if >60s; fails if >120s | `-short` flag |
| `TestContributorSmoke_ArchitectureDocLinksADRs` | Checks ADR links exist | Individual ADR file absent |
| `TestContributorSmoke_BuiltinsValidatePass` | Runs `builtins.Validate()` | `spec/builtins.yaml` or `internal/builtins` absent |
| `TestContributorSmoke_PackagesHaveDocOrADR` | All internal pkgs documented | None — soft-fail with full list |
| `TestContributorSmoke_NoDestructiveDefaults` | No files written outside safe dirs | `-short` flag |

### ADR dependency matrix

| ADR | Status |
|-----|--------|
| `docs/architecture/builtins-registry.md` | Present |
| `docs/architecture/wizard-onboarding.md` | Present |
| `docs/architecture/kit-bus-adoption.md` | Present |
| `spec/builtins.yaml` registry | Present |
| `internal/builtins.Validate()` | Not created; `TestContributorSmoke_BuiltinsValidatePass` skips |
