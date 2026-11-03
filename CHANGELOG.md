# Changelog

## [0.1.0-alpha.1](https://github.com/hop-top/poly-nerv/compare/nerv/v0.1.0-alpha.0...nerv/v0.1.0-alpha.1) (2026-11-03)


### ⚠ BREAKING CHANGES

* **dispatch:** canonical wrapped rewrite {tool_input}

### Features

* **adapters:** deliver decision context on codex, opencode, openhands, vibe ([0bb5626](https://github.com/hop-top/poly-nerv/commit/0bb5626d19eaf4fce34603c7a76ebc4dbedd7a37))
* **dispatch:** canonical wrapped rewrite {tool_input} ([886b7e8](https://github.com/hop-top/poly-nerv/commit/886b7e89bf31f96bef9736902514001a7a0527da))
* **install:** multi-file adapter support in transactional installer ([15c73a2](https://github.com/hop-top/poly-nerv/commit/15c73a29dba832691f579e3eb52a0edb1f5db46e))
* **nervd:** --adapter routing for stateless hook I/O ([4a63468](https://github.com/hop-top/poly-nerv/commit/4a634689aa1186915af86bae74bc314b4f1242df))
* **opencode:** honor rewrite on tool.execute.before ([cd4dc14](https://github.com/hop-top/poly-nerv/commit/cd4dc14833cdedc325446d1a7267f61a3890cd1d))
* **opencode:** v2 plugin bridge; fix install of multi-file adapter ([f44b033](https://github.com/hop-top/poly-nerv/commit/f44b0337b45aac89237a54c4adc43fc9cd832b16))
* **protocol:** optional decision context for model-context channels ([e8615ea](https://github.com/hop-top/poly-nerv/commit/e8615eadace6b7aba09fd1432a64eabe341150a4))
* **sdk/sh:** nerv_context helper for model-facing decision context ([502e8af](https://github.com/hop-top/poly-nerv/commit/502e8aff7aacbf2f80b0d06955bf72676630956c))


### Bug Fixes

* **aider:** hooks call nervd --adapter aider ([9668307](https://github.com/hop-top/poly-nerv/commit/9668307879010efcac317ea191e05eb36761b70a))
* **aider:** stop claiming session hook capabilities ([04820e7](https://github.com/hop-top/poly-nerv/commit/04820e7ad4cf0c5202ce7a61ee97ed3dccf25938))
* **codex:** preserve unknown keys in hooks.json merge ([8caceb7](https://github.com/hop-top/poly-nerv/commit/8caceb790c0413040e46dfa18a80bba2a99b1e64))
* **dispatch:** event log write failure keeps merged decision ([9b57401](https://github.com/hop-top/poly-nerv/commit/9b57401fe4e404425bf7bbe33c6cb22dda686569))
* **docker:** install goose from pinned github release ([4b5805b](https://github.com/hop-top/poly-nerv/commit/4b5805b0406e9fbb7548fa02da7e5c4eba6a2333))
* **goose:** claim only delivered plugin hooks in capabilities ([0db55ef](https://github.com/hop-top/poly-nerv/commit/0db55ef4922b7216d7d2f1c78cc59ef88553a0e3))
* **goose:** install plugin hooks calling nervd --adapter goose ([979d0c4](https://github.com/hop-top/poly-nerv/commit/979d0c4fb5c7e1e47b0018f0705f4af07d67d5aa))
* **install:** gemini and codex hooks call nervd --adapter ([1c5a4e2](https://github.com/hop-top/poly-nerv/commit/1c5a4e21003eca9209478ad4bff55b555081e969))
* **lint:** return typed error not os.Exit; locate hook lines by yaml node ([0b0abc8](https://github.com/hop-top/poly-nerv/commit/0b0abc854cb03fd99af7abcffb2b43f1b0807ef9))
* **nervd:** check-free stderr writes on injected writer ([75f64f9](https://github.com/hop-top/poly-nerv/commit/75f64f95b146c8f3fd210df3f24c8f782796db7d))
* **nerv:** doctor not-executable diagnosis; tail exits on ctx, --no-follow ([35dc94b](https://github.com/hop-top/poly-nerv/commit/35dc94b4c5b953a2bea375d4bdaabe7255d4127a))
* **opencode:** merge opencode.json in MergeConfig, not plugin source ([f903a55](https://github.com/hop-top/poly-nerv/commit/f903a5587c6d13f3bc72defe9c97e3a87cf63bc1))
* **opencode:** route bus events via event hook; pin plugin API v1 ([8088664](https://github.com/hop-top/poly-nerv/commit/80886642118958b9802b590dffcbeb391d163093))
* **release:** declare linked-versions as plugin ([bb43e04](https://github.com/hop-top/poly-nerv/commit/bb43e04d30d06d4977411861b305cecfcfdfc4b2))

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
