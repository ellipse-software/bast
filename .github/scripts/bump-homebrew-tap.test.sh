#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
stable_script="$repo_root/.github/scripts/bump-homebrew-tap.sh"
nightly_script="$repo_root/.github/scripts/bump-homebrew-tap-nightly.sh"
test_root="$(mktemp -d)"

cleanup() {
  rm -rf "$test_root"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_file_contains() {
  local needle="$1"
  local file="$2"
  grep -Fqe "$needle" "$file" || fail "expected $file to contain: $needle"
}

assert_file_lacks() {
  local needle="$1"
  local file="$2"
  if grep -Fqe "$needle" "$file"; then
    fail "expected $file not to contain: $needle"
  fi
}

assert_fail() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    fail "$label: expected failure"
  fi
}

archive="$test_root/source.tar.gz"
printf 'bast source fixture\n' > "$test_root/payload"
tar -czf "$archive" -C "$test_root" payload
expected_sha="$(shasum -a 256 "$archive" | awk '{print $1}')"

stable_tap="$test_root/stable-tap"
TAP_DIR="$stable_tap" SOURCE_ARCHIVE="$archive" bash "$stable_script" v0.9.2

stable_formula="$stable_tap/Formula/bast.rb"
test -f "$stable_formula" || fail "stable formula was not written"
assert_file_contains 'url "https://github.com/ellipse-software/bast/archive/refs/tags/v0.9.2.tar.gz"' "$stable_formula"
assert_file_contains "sha256 \"$expected_sha\"" "$stable_formula"
assert_file_contains 'depends_on "go" => :build' "$stable_formula"
assert_file_contains 'conflicts_with "bast-nightly"' "$stable_formula"
assert_file_contains 'system "go", "build", *std_go_args(ldflags:)' "$stable_formula"
assert_file_contains 'cd "apps/bast" do' "$stable_formula"
assert_file_contains 'ENV["BAST_NO_TELEMETRY"] = "1"' "$stable_formula"
assert_file_contains 'head "https://github.com/ellipse-software/bast.git", branch: "master"' "$stable_formula"
assert_file_lacks 'darwin_arm64.tar.gz' "$stable_formula"
assert_file_lacks '{{' "$stable_formula"
if grep -q '^  version "' "$stable_formula"; then
  fail "stable formula should not set version when the tag URL already includes it"
fi

assert_fail "stable tag without v prefix" env TAP_DIR="$test_root/bad" SOURCE_ARCHIVE="$archive" bash "$stable_script" 0.9.2
assert_fail "stable missing archive" env TAP_DIR="$test_root/bad" SOURCE_ARCHIVE="$test_root/missing.tar.gz" bash "$stable_script" v0.9.2

commit="208d07040fae687d13115abad045115f8ca3baa2"
nightly_tap="$test_root/nightly-tap"
TAP_DIR="$nightly_tap" SOURCE_ARCHIVE="$archive" SOURCE_COMMIT="$commit" \
  bash "$nightly_script" nightly.20260819.208d070

nightly_formula="$nightly_tap/Formula/bast-nightly.rb"
test -f "$nightly_formula" || fail "nightly formula was not written"
assert_file_contains "url \"https://github.com/ellipse-software/bast/archive/${commit}.tar.gz\"" "$nightly_formula"
assert_file_contains "sha256 \"$expected_sha\"" "$nightly_formula"
assert_file_contains 'version "nightly.20260819.208d070"' "$nightly_formula"
assert_file_contains 'depends_on "go" => :build' "$nightly_formula"
assert_file_contains 'conflicts_with "bast"' "$nightly_formula"
assert_file_contains 'system "go", "build", *std_go_args(ldflags:)' "$nightly_formula"
version_line="$(grep -n '^  version "' "$nightly_formula" | head -n1 | cut -d: -f1)"
sha_line="$(grep -n '^  sha256 "' "$nightly_formula" | head -n1 | cut -d: -f1)"
depends_line="$(grep -n 'depends_on "go"' "$nightly_formula" | head -n1 | cut -d: -f1)"
conflicts_line="$(grep -n 'conflicts_with "bast"' "$nightly_formula" | head -n1 | cut -d: -f1)"
if [[ "$version_line" -ge "$sha_line" ]]; then
  fail "nightly version must come before sha256"
fi
if [[ "$depends_line" -ge "$conflicts_line" ]]; then
  fail "depends_on must come before conflicts_with"
fi
assert_file_lacks 'darwin_arm64.tar.gz' "$nightly_formula"
assert_file_lacks '{{' "$nightly_formula"

assert_fail "nightly version without prefix" \
  env TAP_DIR="$test_root/bad" SOURCE_ARCHIVE="$archive" SOURCE_COMMIT="$commit" \
  bash "$nightly_script" 20260819.208d070
assert_fail "nightly commit sha mismatch" \
  env TAP_DIR="$test_root/bad" SOURCE_ARCHIVE="$archive" SOURCE_COMMIT="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" \
  bash "$nightly_script" nightly.20260819.208d070
assert_fail "nightly short commit" \
  env TAP_DIR="$test_root/bad" SOURCE_ARCHIVE="$archive" SOURCE_COMMIT="208d070" \
  bash "$nightly_script" nightly.20260819.208d070
assert_fail "nightly missing commit and token" \
  env TAP_DIR="$test_root/bad" SOURCE_ARCHIVE="$archive" SOURCE_COMMIT="" GH_TOKEN="" \
  bash "$nightly_script" nightly.20260819.208d070

echo "OK"
