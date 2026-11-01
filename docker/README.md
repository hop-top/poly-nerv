# Test Docker Infrastructure

Parameterized Docker setup for running nerv conformance tests against
multiple CLI tools and versions.

## Files

| File | Purpose |
|------|---------|
| `Dockerfile.test` | Single parameterized image; installs CLI + builds nerv |
| `docker-compose.test.yaml` | Local multi-service orchestration |
| `../versions.yaml` | Version matrix consumed by CI and compose |

## Build args

| Arg | Required | Default | Description |
|-----|----------|---------|-------------|
| `CLI_NAME` | yes | - | Binary name (`claude`, `gemini`, `codex`, `opencode`, `goose`, `openhands`) |
| `CLI_KEY` | no | `CLI_NAME` | `versions.yaml` matrix key; set for `opencode-v2` |
| `CLI_VERSION` | no | last in `versions.yaml` | Semver string or `latest` (`github-release` needs a pinned version) |

Package, install method and `github-release` sha256 pins come from
`../versions.yaml` via `../scripts/install-cli.sh` (shared with the
record workflow).

## Usage

### Single CLI

```sh
docker build -f docker/Dockerfile.test \
  --build-arg CLI_NAME=claude \
  --build-arg CLI_VERSION=1.2.1 \
  -t nerv-test-claude .
docker run nerv-test-claude
```

### All CLIs via compose

```sh
docker compose -f docker/docker-compose.test.yaml build
docker compose -f docker/docker-compose.test.yaml up
```

Override versions with env vars:

```sh
CLAUDE_VERSION=1.1.4 docker compose -f docker/docker-compose.test.yaml up claude
```

## Image contents

- **Base**: `node:24.22.1-slim` + Go 1.27.3 toolchain (multi-stage copy)
- **System deps**: git, jq, curl, ca-certificates
- **nerv binaries**: `nerv` and `nervd` built from source at `/usr/local/bin/`
- **Entrypoint**: `go test -v -count=1` with conformance test defaults
