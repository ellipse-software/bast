#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
test_root="$(mktemp -d)"
dist_dir="$test_root/dist"
output_dir="$test_root/output"

cleanup() {
  rm -rf "$test_root"
}
trap cleanup EXIT

mkdir -p "$dist_dir"
for architecture in amd64 arm64; do
  archive="bast_0.9.0_windows_${architecture}.zip"
  printf '%s' "$architecture fixture" > "$dist_dir/$archive"
  (cd "$dist_dir" && shasum -a 256 "$archive" > "$archive.sha256")
done

cd "$repo_root"
bash .github/scripts/render-winget-manifest.sh v0.9.0 "$dist_dir" "$output_dir"

assert_line() {
  local expected="$1"
  local file="$2"
  grep -Fqx "$expected" "$file"
}

assert_line \
  '# yaml-language-server: $schema=https://aka.ms/winget-manifest.installer.1.12.0.schema.json' \
  "$output_dir/EllipseSoftware.Bast.installer.yaml"
assert_line \
  '# yaml-language-server: $schema=https://aka.ms/winget-manifest.defaultLocale.1.12.0.schema.json' \
  "$output_dir/EllipseSoftware.Bast.locale.en-US.yaml"
assert_line \
  '# yaml-language-server: $schema=https://aka.ms/winget-manifest.version.1.12.0.schema.json' \
  "$output_dir/EllipseSoftware.Bast.yaml"

for manifest in "$output_dir"/*.yaml; do
  assert_line 'PackageVersion: 0.9.0' "$manifest"
  assert_line 'ManifestVersion: 1.12.0' "$manifest"
  if grep -Fq '{{' "$manifest"; then
    echo "Unresolved template value in $manifest" >&2
    exit 1
  fi
done

for architecture in amd64 arm64; do
  archive="bast_0.9.0_windows_${architecture}.zip"
  expected_hash="$(shasum -a 256 "$dist_dir/$archive" | awk '{ print toupper($1) }')"
  assert_line "    InstallerSha256: $expected_hash" "$output_dir/EllipseSoftware.Bast.installer.yaml"
done
