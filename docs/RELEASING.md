# Releasing nerv

nerv is a polyglot project. Releases are cut by
[release-please](https://github.com/googleapis/release-please) over six
linked components (group `nerv`), then fanned out to their package
ecosystems by the
[`hop-top/.github` `publish-on-tag`](https://github.com/hop-top/.github)
reusable workflow (pinned to a release commit SHA).

## Version scheme

- `0.1.0-alpha.N` — current channel (polyglot alpha)
- `0.x.y-alpha.N` — alpha channel until the polyglot surface is stable
- `0.x.y` — beta / stable once the spec is ratified
- `1.0.0` — first stable

All six components share the same version. release-please's
`linked-versions` group `nerv` enforces this: whenever one component
bumps, all six entries in `.release-please-manifest.json` move to the
same version. `separate-pull-requests` is on, so each component gets its
own release PR and its own tag.

## Components

| Component  | Path       | Ecosystem | Package           | Mirror                  |
|------------|------------|-----------|-------------------|-------------------------|
| `nerv`     | `.`        | Go        | `hop.top/nerv`    | `hop-top/nerv`          |
| `nerv-go`  | `sdk/go`   | Go        | `hop.top/nerv/sdk/go` | `hop-top/nerv` (tag only) |
| `nerv-ts`  | `sdk/ts`   | npm       | `@hop-top/nerv`   | `hop-top/nerv-ts`       |
| `nerv-py`  | `sdk/py`   | PyPI      | `nerv-sdk`        | `hop-top/nerv-py`       |
| `nerv-rs`  | `sdk/rs`   | crates.io | `hop-top-nerv`    | `hop-top/nerv-rs`       |
| `nerv-php` | `sdk/php`  | Packagist | `hop-top/nerv`    | `hop-top/nerv-php`      |

Mirrors are read-only; they receive a subtree push on tag (except
`nerv-go`, see below) so the
ecosystem-specific tooling (proxy.golang.org, Packagist webhooks, etc.)
can resolve the package against a single-language repo.

`sdk/php/composer.json` has no `version` field, as Packagist
recommends for VCS packages: Packagist reads the version from the
mirror tag.

### Go modules

The repo holds two Go modules, both served from the `hop-top/nerv`
mirror. The root module is mirrored at the mirror root; `sdk/go` is a
nested module in that tree, so it needs only a prefixed tag, which is
how the Go toolchain versions a module in a subdirectory:

| Module                | Monorepo tag        | Mirror tag        | Mirror         |
|-----------------------|---------------------|-------------------|----------------|
| `hop.top/nerv`        | `nerv/vX.Y.Z`       | `vX.Y.Z`          | `hop-top/nerv` |
| `hop.top/nerv/sdk/go` | `nerv-go/vX.Y.Z`    | `sdk/go/vX.Y.Z`   | `hop-top/nerv` |

`nerv-go` uses `mirror-mode: tag-only`: publish-on-tag pushes only the
`sdk/go/vX.Y.Z` tag, on the mirror commit that holds this release's
`sdk/go` tree. The root mirror push must land first, so tag `nerv` no
later than `nerv-go`.

The `hop.top` vanity server must answer `?go-get=1` for `hop.top/nerv`
(and every package path under it, `hop.top/nerv/sdk/go/...` included)
with a `go-import` meta tag naming `hop-top/nerv`.

`sdk/go/go.mod` requires `hop.top/nerv` at the release version;
release-please rewrites that line (`x-release-please-version`) in the
same release PR. Its `replace hop.top/nerv => ../..` is for local
development only; Go ignores it for consumers. Because the SDK requires
the root module at the same version, tag the root (`nerv/vX.Y.Z`) no
later than the SDK.

## Release flow

1. Land work into `main` with [Conventional Commits](https://www.conventionalcommits.org/).
   `feat:` and `fix:` drive version bumps; the changelog sections map per
   `.github/release-please-config.json`.
2. On every push to `main`,
   `.github/workflows/release-please.yml` mints a short-lived token from
   the `hop-top/release-bot` GitHub App and runs
   `googleapis/release-please-action` (v5, SHA-pinned).
3. release-please opens (or updates) a single PR titled
   `chore(release):<component> <version>` per component when a bump is
   warranted. Because the group is linked, all six PRs land together
   when one component bumps — review and merge as one set.
4. Merging the release PR(s) creates six tags:
   `nerv/v<version>`, `nerv-go/v<version>`, `nerv-ts/v<version>`, …, `nerv-php/v<version>`.
5. Each `*/v*` tag triggers `.github/workflows/publish.yml`, which calls
   `hop-top/.github/.github/workflows/publish-on-tag.yml` (SHA-pinned) with the
   ecosystems map. That reusable workflow:
   - Runs each component's test suite from its mirror checkout.
   - For `ecosystem: go`, runs no publish job: the module is served
     from the mirror tag through the Go module proxy. No binary
     archives are attached; users install with `go install`.
   - For `npm` / `pypi` / `crates` / `packagist`, publishes the package
     under the configured secret (npm + crates: token; PyPI: token —
     OIDC pending upstream debug; Packagist: webhook).
   - Subtree-pushes each component to its mirror under `hop-top/<name>`.

## App-token pattern

release-please runs as `release-bot[bot]`, not as the human owner. This
avoids the CODEOWNERS self-approval block on
`.release-please-manifest.json` and matches the
`poly-kit` / `vein` polyglot precedent.

Secrets required on the repository:

- `RELEASE_BOT_APP_ID` — GitHub App ID for `hop-top/release-bot`
- `RELEASE_BOT_PRIVATE_KEY` — App private key (PEM)
- `NPM_REGISTRY_TOKEN`, `CARGO_REGISTRY_TOKEN`, `PYPI_REGISTRY_TOKEN`,
  `PACKAGIST_USERNAME`, `PACKAGIST_TOKEN`, `GH_MIRROR_PAT` — consumed by
  `publish-on-tag` per ecosystem.

## First-tag dry run (local)

Before cutting `0.1.0-alpha.0` on the live remote, run release-please in
dry-run locally to confirm the manifest seed + component plumbing
behaves:

```bash
npx --yes release-please@latest manifest-pr \
  --token "$GITHUB_TOKEN" \
  --repo-url https://github.com/hop-top/poly-nerv \
  --config-file .github/release-please-config.json \
  --manifest-file .github/.release-please-manifest.json \
  --dry-run
```

Expected output: six `chore(release):<component> <version>` PR
descriptions; the `nerv-go` one also bumps the `hop.top/nerv` require
in `sdk/go/go.mod`.

## Post-release

- `versions.yaml` — update on conformance-record nightly to track latest
  upstream CLI versions.
- `nerv doctor` — re-run on a fresh install to confirm the released
  binaries report the new version and adapter shims load.
- Mirrors — confirm `hop-top/<component>` updated and the ecosystem
  registries (`pkg.go.dev`, npm, PyPI, crates.io, Packagist) resolved
  the new tag.
