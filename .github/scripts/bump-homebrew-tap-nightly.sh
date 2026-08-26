#!/usr/bin/env bash
set -euo pipefail

version="${1:?usage: bump-homebrew-tap-nightly.sh <version> [commit]}"
commit="${2:-${SOURCE_COMMIT:-}}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
template="$repo_root/.github/homebrew/bast-nightly.rb.template"

bast_repo="ellipse-software/bast"
tap_repo="ellipse-software/homebrew-tap"

if [[ "$version" != nightly.* ]]; then
  echo "version must start with nightly. (got: $version)" >&2
  exit 1
fi

if [[ ! -f "$template" ]]; then
  echo "missing formula template: $template" >&2
  exit 1
fi

short_sha="${version##*.}"
if [[ -z "$commit" ]]; then
  if [[ -z "${GH_TOKEN:-}" ]]; then
    echo "pass a commit SHA or set GH_TOKEN to resolve ${short_sha}" >&2
    exit 1
  fi
  commit="$(
    curl -fsSL \
      -H "Authorization: Bearer ${GH_TOKEN}" \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      "https://api.github.com/repos/${bast_repo}/commits/${short_sha}" \
      | python3 -c 'import json, sys; print(json.load(sys.stdin)["sha"])'
  )"
fi

if [[ ! "$commit" =~ ^[0-9a-fA-F]{40}$ ]]; then
  echo "commit must be a 40-character SHA (got: $commit)" >&2
  exit 1
fi

if [[ "${commit:0:7}" != "$short_sha" ]]; then
  echo "commit ${commit} does not match nightly version SHA ${short_sha}" >&2
  exit 1
fi

archive="${SOURCE_ARCHIVE:-}"
cleanup_archive=0
if [[ -z "$archive" ]]; then
  archive="$(mktemp "${TMPDIR:-/tmp}/bast-nightly-source.XXXXXX")"
  cleanup_archive=1
  curl -fsSL "https://github.com/${bast_repo}/archive/${commit}.tar.gz" -o "$archive"
fi

cleanup() {
  if [[ "$cleanup_archive" -eq 1 ]]; then
    rm -f "$archive"
  fi
}
trap cleanup EXIT

sha256="$(shasum -a 256 "$archive" | awk '{print $1}')"
test -n "$sha256"

write_formula() {
  local outfile="$1"
  sed \
    -e "s/{{COMMIT}}/${commit}/g" \
    -e "s/{{SHA256}}/${sha256}/g" \
    -e "s/{{VERSION}}/${version}/g" \
    "$template" > "$outfile"
  if grep -q '{{' "$outfile"; then
    echo "unresolved template placeholder in $template" >&2
    exit 1
  fi
}

if [[ -n "${TAP_DIR:-}" ]]; then
  mkdir -p "${TAP_DIR}/Formula"
  write_formula "${TAP_DIR}/Formula/bast-nightly.rb"
  echo "Wrote ${TAP_DIR}/Formula/bast-nightly.rb for ${version}"
  exit 0
fi

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "GH_TOKEN is required" >&2
  exit 1
fi

tap_dir="$(mktemp -d "${TMPDIR:-/tmp}/homebrew-tap.XXXXXX")"
cleanup_tap() {
  cleanup
  rm -rf "$tap_dir"
}
trap cleanup_tap EXIT

git clone --depth 1 "https://x-access-token:${GH_TOKEN}@github.com/${tap_repo}.git" "$tap_dir"
write_formula "${tap_dir}/Formula/bast-nightly.rb"

pushd "$tap_dir" >/dev/null
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

if git diff --quiet; then
  echo "Formula already up to date for ${version}"
  exit 0
fi

git add Formula/bast-nightly.rb
git commit -m "bast-nightly ${version}"
git push origin HEAD
popd >/dev/null

echo "Updated ${tap_repo} bast-nightly to ${version}"
