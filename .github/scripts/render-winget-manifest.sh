#!/usr/bin/env bash
set -euo pipefail

tag="${1:?release tag is required}"
dist_dir="${2:-dist}"
output_dir="${3:-$dist_dir}"
version="${tag#v}"

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "WinGet manifests require a stable vX.Y.Z tag" >&2
  exit 1
fi

hash_for() {
  local archive="$1"
  local checksum="$dist_dir/$archive.sha256"
  test -f "$dist_dir/$archive"
  test -f "$checksum"
  if ! (cd "$dist_dir" && shasum -a 256 -c "$archive.sha256" >/dev/null); then
    echo "Checksum verification failed for $archive" >&2
    return 1
  fi
  awk -v archive="$archive" '$2 == archive || $2 == "*" archive { print toupper($1); exit }' "$checksum"
}

amd64_archive="bast_${version}_windows_amd64.zip"
arm64_archive="bast_${version}_windows_arm64.zip"
amd64_hash="$(hash_for "$amd64_archive")"
arm64_hash="$(hash_for "$arm64_archive")"
test -n "$amd64_hash"
test -n "$arm64_hash"

mkdir -p "$output_dir"
for template in .github/winget/*.yaml.template; do
  output="$output_dir/$(basename "$template" .template)"
  sed \
    -e "s/{{VERSION}}/$version/g" \
    -e "s/{{AMD64_SHA256}}/$amd64_hash/g" \
    -e "s/{{ARM64_SHA256}}/$arm64_hash/g" \
    "$template" > "$output"
done
