#!/usr/bin/env bash
set -euo pipefail

tag="${1:?usage: package-linux.sh <tag> <dist_dir>}"
dist_dir="${2:-dist}"
version="${tag#v}"
NFPM_VERSION="${NFPM_VERSION:-2.47.0}"

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Linux packages require a stable vX.Y.Z tag (got: $tag)" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
config="$repo_root/.github/nfpm/bast.yaml"
license="$repo_root/LICENSE"
readme="$repo_root/apps/bast/README.md"

if [[ ! -f "$config" ]]; then
  echo "missing nFPM config: $config" >&2
  exit 1
fi
if [[ ! -f "$license" || ! -f "$readme" ]]; then
  echo "missing LICENSE or README" >&2
  exit 1
fi

if [[ ! -d "$dist_dir" ]]; then
  echo "dist directory does not exist: $dist_dir" >&2
  exit 1
fi

hash_file() {
  local file="$1"
  if command -v shasum >/dev/null 2>&1; then
    (cd "$(dirname "$file")" && shasum -a 256 "$(basename "$file")")
  else
    (cd "$(dirname "$file")" && sha256sum "$(basename "$file")")
  fi
}

verify_archive() {
  local archive="$1"
  local checksum="$archive.sha256"
  if [[ ! -f "$archive" ]]; then
    echo "missing archive: $archive" >&2
    return 1
  fi
  if [[ ! -f "$checksum" ]]; then
    echo "missing checksum: $checksum" >&2
    return 1
  fi
  local dir
  dir="$(cd "$(dirname "$archive")" && pwd)"
  local name
  name="$(basename "$archive")"
  if command -v shasum >/dev/null 2>&1; then
    (cd "$dir" && shasum -a 256 -c "$name.sha256") >/dev/null
  else
    (cd "$dir" && sha256sum -c "$name.sha256") >/dev/null
  fi
}

tmp_root="${TMPDIR:-/tmp}"
tmp_root="${tmp_root%/}"
work="$(mktemp -d "${tmp_root}/bast-nfpm.XXXXXX")"
cleanup() {
  rm -rf "$work"
}
trap cleanup EXIT

install_nfpm() {
  if command -v nfpm >/dev/null 2>&1; then
    return 0
  fi

  local os arch url
  case "$(uname -s)" in
  Linux) os="Linux" ;;
  Darwin) os="Darwin" ;;
  *)
    echo "nfpm is not installed and cannot be fetched on $(uname -s)" >&2
    return 1
    ;;
  esac
  case "$(uname -m)" in
  x86_64 | amd64) arch="x86_64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *)
    echo "nfpm is not installed and cannot be fetched for $(uname -m)" >&2
    return 1
    ;;
  esac

  url="https://github.com/goreleaser/nfpm/releases/download/v${NFPM_VERSION}/nfpm_${NFPM_VERSION}_${os}_${arch}.tar.gz"
  mkdir -p "$work/bin"
  curl -fsSL "$url" | tar -xz -C "$work/bin" nfpm
  chmod +x "$work/bin/nfpm"
  export PATH="$work/bin:$PATH"
}

install_nfpm

write_checksum() {
  local file="$1"
  hash_file "$file" >"$file.sha256"
}

package_arch() {
  local goarch="$1"
  local bundle="bast_${version}_linux_${goarch}"
  local archive="$dist_dir/${bundle}.tar.gz"

  verify_archive "$archive"

  local extract="$work/$goarch"
  mkdir -p "$extract"
  tar -xzf "$archive" -C "$extract"

  local bin="$extract/$bundle/bast"
  if [[ ! -f "$bin" ]]; then
    echo "archive $archive did not contain $bundle/bast" >&2
    return 1
  fi
  chmod +x "$bin"
  bin="$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")"

  export GOARCH="$goarch"
  export VERSION="$version"
  export BAST_BIN="$bin"
  export BAST_LICENSE="$license"
  export BAST_README="$readme"

  local packager target
  for packager in deb rpm apk archlinux; do
    case "$packager" in
    deb) target="$dist_dir/bast_${version}_linux_${goarch}.deb" ;;
    rpm) target="$dist_dir/bast_${version}_linux_${goarch}.rpm" ;;
    apk) target="$dist_dir/bast_${version}_linux_${goarch}.apk" ;;
    archlinux) target="$dist_dir/bast_${version}_linux_${goarch}.pkg.tar.zst" ;;
    esac
    nfpm package --config "$config" --packager "$packager" --target "$target"
    write_checksum "$target"
  done
}

package_arch amd64
package_arch arm64
