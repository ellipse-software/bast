#!/usr/bin/env bash
set -euo pipefail

tag="${1:?usage: bump-homebrew-tap.sh <tag>}"
version="${tag#v}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
template="$repo_root/.github/homebrew/bast.rb.template"

bast_repo="ellipse-software/bast"
tap_repo="ellipse-software/homebrew-tap"

if [[ "$tag" != v* ]]; then
  echo "tag must start with v (got: $tag)" >&2
  exit 1
fi

if [[ ! -f "$template" ]]; then
  echo "missing formula template: $template" >&2
  exit 1
fi

archive="${SOURCE_ARCHIVE:-}"
cleanup_archive=0
if [[ -z "$archive" ]]; then
  archive="$(mktemp "${TMPDIR:-/tmp}/bast-source.XXXXXX")"
  cleanup_archive=1
  curl -fsSL "https://github.com/${bast_repo}/archive/refs/tags/${tag}.tar.gz" -o "$archive"
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
    -e "s/{{TAG}}/${tag}/g" \
    -e "s/{{SHA256}}/${sha256}/g" \
    "$template" > "$outfile"
  if grep -q '{{' "$outfile"; then
    echo "unresolved template placeholder in $template" >&2
    exit 1
  fi
}

if [[ -n "${TAP_DIR:-}" ]]; then
  mkdir -p "${TAP_DIR}/Formula"
  write_formula "${TAP_DIR}/Formula/bast.rb"
  echo "Wrote ${TAP_DIR}/Formula/bast.rb for ${tag}"
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
write_formula "${tap_dir}/Formula/bast.rb"

pushd "$tap_dir" >/dev/null
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

if git diff --quiet; then
  echo "Formula already up to date for ${tag}"
  exit 0
fi

git add Formula/bast.rb
git commit -m "bast ${tag}"
git push origin HEAD
popd >/dev/null

echo "Updated ${tap_repo} to ${tag} (version ${version})"
