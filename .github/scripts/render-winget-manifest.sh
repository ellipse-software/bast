#!/usr/bin/env bash
set -euo pipefail

tag="${1:?release tag is required}"
dist_dir="${2:-dist}"
output="${3:-$dist_dir/EllipseSoftware.Bast.yaml}"
version="${tag#v}"

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "WinGet manifests require a stable vX.Y.Z tag" >&2
  exit 1
fi

hash_for() {
  local archive="$1"
  local checksum="$dist_dir/$archive.sha256"
  test -f "$checksum"
  awk -v archive="$archive" '$2 == archive || $2 == "*" archive { print toupper($1); exit }' "$checksum"
}

amd64_archive="bast_${version}_windows_amd64.zip"
arm64_archive="bast_${version}_windows_arm64.zip"
amd64_hash="$(hash_for "$amd64_archive")"
arm64_hash="$(hash_for "$arm64_archive")"
test -n "$amd64_hash"
test -n "$arm64_hash"

mkdir -p "$(dirname "$output")"
sed \
  -e "s/{{VERSION}}/$version/g" \
  -e "s/{{AMD64_SHA256}}/$amd64_hash/g" \
  -e "s/{{ARM64_SHA256}}/$arm64_hash/g" \
  .github/winget/EllipseSoftware.Bast.yaml.template > "$output"
