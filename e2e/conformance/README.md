# Conformance grading (12fcc service tier)

Scenario library, recorded cassettes, and captured verdicts for the
service-graded factors of the 12-Factor AI-CLI conformance contract:
F3 Structured I/O, F4 Corrective Error Model, F5 Explicit Contracts,
F6 Previewability, F7 Idempotency, F8 State Transparency, and F11
Exit Code Semantics. The story-graded factors (F1, F2, F9, F12) and
leak scanning (F10) are covered by the existing `12fcc` workflow.

## Layout

```
scenarios/nerv/<id>/1.0.0/scenario.yaml   graded rubric, one per scenario
stories/<id>.yaml                         user story each scenario is bound to
cassettes/<id>/                           captures from real binary runs
  manifest.yaml                           upload manifest (story hash, steps)
  story.yaml                              byte-exact copy of the story
  steps/<step-id>/result.json             exit code + duration as observed
  steps/<step-id>/stdout.txt              stdout as observed
  steps/<step-id>/stderr.txt              stderr as observed
verdicts/<id>.json                        grading service verdict, tier 3
```

Cassettes are recorded, never authored: every stdout/stderr byte and
exit code comes from executing the actual binary. Scenarios encode the
spec-correct expectation even where current behavior violates it — a
failing verdict is the honest outcome, not a broken pipeline.

## Re-running

Record fresh cassettes (builds the binary, runs every scenario step in
an isolated temp project):

```sh
make 12fcc-record
```

Grade them against a locally served scenario library (requires a `kit`
binary that ships the `conformance` command group; point `KIT_BIN` at
one built from `hop.top/kit` `cmd/kit`):

```sh
KIT_BIN=/path/to/kit make 12fcc-grade
```

The grade target boots `kit conformance svc serve` on a loopback port
with `--scenarios-root e2e/conformance`, mints a `grade:nerv` token
into a throwaway claims DB, uploads every cassette at tier 3, rewrites
`verdicts/`, and prints per-scenario verdicts plus a per-factor
rollup. Grading is measurement, not a gate: the target only fails when
a cassette cannot be graded at all. Set `STRICT=1` to also fail on
`fail` verdicts.

## Badge

`.12fc.json` at the repo root — the shields.io endpoint behind the
README badge — is committed and is the source of truth for all 12
factors. CI never rewrites it: the `12fcc` workflow can only run the
story and leak leaves (F1, F2, F9, F10, F12), and the badge verdict
counts measured factors only, so a CI refresh would degrade the
truthful `12/12 pass` badge to a red `5/5 pass` one. The workflow
keeps its badge commit disabled until a released `kit` can genuinely
grade cassettes in CI; the enable checklist lives in
`.github/workflows/12fcc.yaml`.

Refresh the badge locally, from measurements only:

```sh
make 12fcc-record                      # re-record cassettes from the real binary
KIT_BIN=/path/to/kit make 12fcc-grade  # tier-3 verdicts into verdicts/
KIT_BIN=/path/to/kit make 12fcc-badge  # verify leaves + verdicts -> .12fc.json
```

The badge target re-runs `verify-no-leak` and `verify-stories`
against `e2e/stories/` (the same scope CI checks), folds in the
per-factor statuses from `verdicts/`, and renders the badge with
`kit conformance badge` — the same verdict and colour rules CI uses.
Commit the resulting `.12fc.json` diff together with the re-recorded
cassettes and verdicts.

## Current verdicts

As of the captures in `cassettes/` (see `verdicts/` for full facets):

| Scenario | Verdict | Factors |
|---|---|---|
| `status-json` | pass | F3 F8 F11 |
| `doctor-json` | pass | F3 F8 F11 |
| `add-idempotent-reapply` | pass | F7 |
| `add-dry-run-preview` | pass | F3 F6 F11 |
| `error-envelope-correction` | pass | F4 F11 |
| `contract-surfaces` | pass | F5 F11 |

Factor rollup: every graded factor (F3, F4, F5, F6, F7, F8, F11)
passes. The behaviors that flipped the previously failing verdicts:

- **F3/F6** — `add --dry-run` applies nothing and emits a structured
  kit plan (JSON under `--format json`); `add`/`remove`/`daemon
  status` render through `output.Render` on every format.
- **F4** — not-found envelopes carry `suggested_fix` and, when the
  event log has entries, concrete `nerv replay <id>` `alternatives`.
- **F5** — `nerv capabilities` serves the machine-readable command
  surface (leaves with path/name/flags) built on kit's toolspec
  walker.
- **F11** — the process honors envelope-declared exit codes and the
  kit exit-class table (USAGE=2, NOT_FOUND=3, UNAUTHORIZED=5);
  unknown commands are usage errors instead of help + exit 0.

If a regression re-introduces one of these defects, re-record and
re-grade; the scenario library encodes the spec-correct expectation,
so the verdict flips without rubric changes.
