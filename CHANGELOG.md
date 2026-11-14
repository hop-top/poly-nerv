# Changelog

## [0.1.0-alpha.1](https://github.com/hop-top/poly-nerv/compare/nerv/v0.1.0-alpha.0...nerv/v0.1.0-alpha.1) (2026-10-28)


### Bug Fixes

* **aider:** stop claiming session hook capabilities ([04820e7](https://github.com/hop-top/poly-nerv/commit/04820e7ad4cf0c5202ce7a61ee97ed3dccf25938))
* **codex:** preserve unknown keys in hooks.json merge ([8caceb7](https://github.com/hop-top/poly-nerv/commit/8caceb790c0413040e46dfa18a80bba2a99b1e64))
* **opencode:** merge opencode.json in MergeConfig, not plugin source ([f903a55](https://github.com/hop-top/poly-nerv/commit/f903a5587c6d13f3bc72defe9c97e3a87cf63bc1))

## Changelog — nerv

All notable changes to nerv land here. Format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versioning follows
[SemVer](https://semver.org/) with `alpha.N` prerelease channel during 0.x.

This file is owned by release-please from `0.1.0-alpha.0` onward — the
`## Unreleased` block below documents the in-flight changes that will land
in the first tagged release.

## Unreleased

### Project rename

- **`uhp` → `nerv`** — wire-breaking rename across module path, binary
  names, env vars, config dirs, and SDK packages.
  - Module: `hop.top/uhp` → `hop.top/nerv`
  - Binaries: `uhp` → `nerv`, `uhpd` → `nervd`
  - Config dir: `.uhp/` → `.nerv/`
  - Env prefix: `UHP_*` → `NERV_*` (e.g. `UHP_SOCKET` → `NERV_SOCKET`)
  - Wire field: `uhp_version` → `nerv_version` in envelope schema
  - Brand: "Universal Hooks Protocol" replaces "Universal Hook Protocol"
    where applicable; `nerv` is the project name and ships the canonical
    implementation.

### Polyglot SDKs

- **5 SDKs scaffolded under `sdk/`** — first-class client libraries that
  dispatch envelopes through `nervd` over a Unix socket.
  - `sdk/go` — Go client (`hop.top/nerv/sdk/go`)
  - `sdk/ts` — TypeScript / ConnectRPC (`@nerv/sdk`)
  - `sdk/py` — Python / gRPC (`nerv-sdk`)
  - `sdk/rs` — Rust / tonic (`nerv-sdk`)
  - `sdk/php` — PHP / google/protobuf transport stub (`hop-top/nerv`)
- Cross-SDK dispatch parity smoke test landed under `test/parity/`
  exercises Go + TS + Python against a single `nervd` instance.

### Daemon

- **`nervd` refactored to ConnectRPC** — serves the `EventService` over a
  Unix domain socket with `h2c` (HTTP/2 cleartext). `NERV_SOCKET` selects
  the socket path; default lives under `$XDG_RUNTIME_DIR/nerv/`. SDKs in
  every supported language speak the same wire format.
- `nerv daemon start | stop | status | reload` manage the lifecycle;
  `reload` sends `SIGHUP` to hot-swap `hooks.yaml`.

### Release pipeline

- Adopt `release-please` (linked-versions group `nerv` over the 5
  components) gated by a `hop-top/release-bot` App-token mint to bypass
  the CODEOWNERS self-approval block on the manifest.
- Adopt `hop-top/.github` reusable `publish-on-tag@v0` workflow with an
  ecosystems map that fans `*/v*` tags out to Go (goreleaser), npm, PyPI,
  crates.io, and Packagist, with read-only mirror repos under
  `hop-top/<component>`.
- Single-file `release.yaml` removed.
- CI workflow rewired (`ci.yaml` → `ci.yml`) with per-ecosystem jobs
  (`go`, `ts`, `py`, `rs`, `php`) and a `proto` job that runs
  `buf lint` + `buf format -d --exit-code`.
