#!/usr/bin/env bash
# Install one CLI under test, as pinned in versions.yaml.
#
#   scripts/install-cli.sh <key> [version]
#
# key      versions.yaml matrix key (e.g. claude, goose, opencode-v2)
# version  default: last entry of versions.yaml; "latest" allowed for
#          npm/go/pip. github-release needs a pinned, checksummed version.
#
# Methods: npm | go | pip | github-release (sha256 verified against
# versions.yaml; no pin, no install).
#
# Env:
#   INSTALL_DIR        github-release target (default /usr/local/bin)
#   INSTALL_ARCH       override `uname -m` (testing)
#   INSTALL_SKIP_EXEC  1 = skip the `<binary> --version` smoke check
#   VERSIONS_FILE      override versions.yaml path
set -euo pipefail

# yq v4.54.1 (mikefarah/yq), sha256 of the release binaries.
YQ_VERSION=v4.54.1
YQ_SHA256_X86_64=8e34fc298390875de416e6a4afcb8cabeceb25d9aa8506c1a2f9353cf702ea5f
YQ_SHA256_AARCH64=189088da0c6429ec5178dfaab1a114805f6cab0b61b165ab236efedf1d57a71b

die() { echo "install-cli: $*" >&2; exit 1; }

key=${1:-}
[ -n "$key" ] || die "usage: install-cli.sh <key> [version]"
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
vf=${VERSIONS_FILE:-$here/../versions.yaml}
dir=${INSTALL_DIR:-/usr/local/bin}
uarch=${INSTALL_ARCH:-$(uname -m)}
case "$uarch" in arm64) uarch=aarch64 ;; amd64) uarch=x86_64 ;; esac

fetch_verified() { # url sha256 dest
  curl -fsSL --proto '=https' --tlsv1.2 -o "$3" "$1"
  echo "$2  $3" | sha256sum -c - >/dev/null \
    || die "sha256 mismatch for $1"
}

ensure_yq() {
  if command -v yq >/dev/null 2>&1 && yq --version 2>&1 | grep -q mikefarah; then
    return
  fi
  local sum asset=yq_linux_amd64
  case "$uarch" in
    x86_64) sum=$YQ_SHA256_X86_64 ;;
    aarch64) sum=$YQ_SHA256_AARCH64; asset=yq_linux_arm64 ;;
    *) die "unsupported arch $uarch" ;;
  esac
  mkdir -p "$dir"
  fetch_verified \
    "https://github.com/mikefarah/yq/releases/download/$YQ_VERSION/$asset" \
    "$sum" "$dir/yq"
  chmod +x "$dir/yq"
  export PATH="$dir:$PATH"
}
ensure_yq

q() { yq -r "$1" "$vf"; }
base=".matrix.\"$key\""
method=$(q "$base.install // \"\"")
pkg=$(q "$base.package // \"\"")
[ -n "$method" ] || die "no versions.yaml entry for '$key'"
ver=${2:-$(q "$base.versions[-1]")}

case "$method" in
  npm)
    npm i -g "${pkg}@${ver}"
    ;;
  go)
    case "$ver" in latest|v*) ;; *) ver="v$ver" ;; esac
    go install "${pkg}@${ver}"
    ;;
  pip)
    if [ "$ver" = latest ]; then spec=$pkg; else spec="${pkg}==${ver}"; fi
    python3 -m pip install "$spec"
    ;;
  github-release)
    [ "$ver" != latest ] || die "$key: github-release needs a pinned version"
    r="$base.release"
    asset=$(q "$r.asset")
    arch=$(q "$r.arch.\"$uarch\" // \"\"")
    sum=$(q "$r.sha256.\"$ver\".\"$uarch\" // \"\"")
    member=$(q "$r.member")
    bin=$(q "$r.binary")
    [ -n "$arch" ] || die "$key: unsupported arch $uarch"
    [ -n "$sum" ] || die "$key: no sha256 pinned for $ver/$uarch in versions.yaml"
    asset=${asset//\{arch\}/$arch}
    work=$(mktemp -d)
    trap 'rm -rf "$work"' EXIT
    fetch_verified \
      "https://github.com/${pkg}/releases/download/v${ver}/${asset}" \
      "$sum" "$work/cli.tgz"
    mkdir -p "$dir"
    tar -xzf "$work/cli.tgz" -C "$dir" "$member"
    chmod +x "$dir/$bin"
    echo "installed $dir/$bin ($key $ver $uarch, sha256 ok)"
    if [ "${INSTALL_SKIP_EXEC:-0}" != 1 ]; then
      "$dir/$bin" --version
    fi
    ;;
  *)
    die "$key: unsupported install method '$method'"
    ;;
esac
