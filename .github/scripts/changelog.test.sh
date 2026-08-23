#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
script="$repo_root/.github/scripts/changelog.sh"
test_root="$(mktemp -d)"

cleanup() {
  rm -rf "$test_root"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local expected="$1"
  local actual="$2"
  local label="$3"
  if [[ "$actual" != "$expected" ]]; then
    echo "FAIL: $label" >&2
    diff -u <(printf '%s\n' "$expected") <(printf '%s\n' "$actual") >&2 || true
    exit 1
  fi
}

assert_fail() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    fail "$label: expected failure"
  fi
}

changelog="$test_root/CHANGELOG.md"
export CHANGELOG_PATH="$changelog"

write_fixture() {
  cat > "$changelog" <<'EOF'
# Changelog

Intro.

## Unreleased

### Added

- New sync tab.

## v0.10.0-rc.1 - 2026-08-01

Release candidate notes.

## Highlights from rc

- Should stay in the rc section.

## v0.9.0 - 2026-08-19

Bast v0.9.0 adds native Windows 11 support.

## Highlights

- Native Windows builds

## Install

```bash
curl -fsSL https://bast.sh/install | sh
```
EOF
}

write_fixture

got="$(bash "$script" extract v0.9.0)"
expected="$(cat <<'EOF'
Bast v0.9.0 adds native Windows 11 support.

## Highlights

- Native Windows builds

## Install

```bash
curl -fsSL https://bast.sh/install | sh
```
EOF
)"
assert_eq "$expected" "$got" "extract v0.9.0 includes inner ## headings"

got="$(bash "$script" extract v0.10.0-rc.1)"
expected="$(cat <<'EOF'
Release candidate notes.

## Highlights from rc

- Should stay in the rc section.
EOF
)"
assert_eq "$expected" "$got" "extract rc tag is exact"

assert_fail "extract missing tag" bash "$script" extract v0.10.0
assert_fail "extract unknown tag" bash "$script" extract v1.0.0

cat > "$changelog" <<'EOF'
# Changelog

## Unreleased

## v0.9.0 - 2026-08-19

Notes.
EOF
assert_fail "extract empty Unreleased" bash "$script" extract Unreleased
assert_fail "cut empty Unreleased" env CHANGELOG_DATE=2026-08-23 bash "$script" cut v0.10.0

write_fixture
assert_fail "cut duplicate version" env CHANGELOG_DATE=2026-08-23 bash "$script" cut v0.9.0
assert_fail "cut invalid tag" bash "$script" cut nightly.1
assert_fail "cut missing v prefix" bash "$script" cut 0.10.0

write_fixture
CHANGELOG_DATE=2026-08-23 bash "$script" cut v0.10.0

got="$(bash "$script" extract v0.10.0)"
expected="$(cat <<'EOF'
### Added

- New sync tab.
EOF
)"
assert_eq "$expected" "$got" "cut moves Unreleased into the new version"

got="$(head -n 15 "$changelog")"
expected="$(cat <<'EOF'
# Changelog

Intro.

## Unreleased

## v0.10.0 - 2026-08-23

### Added

- New sync tab.

## v0.10.0-rc.1 - 2026-08-01

Release candidate notes.
EOF
)"
assert_eq "$expected" "$got" "cut rewrites headings and leaves Unreleased empty"

assert_fail "extract empty Unreleased after cut" bash "$script" extract Unreleased
assert_fail "second cut with empty Unreleased" env CHANGELOG_DATE=2026-08-24 bash "$script" cut v0.10.1

got="$(bash "$script" extract v0.9.0)"
expected="$(cat <<'EOF'
Bast v0.9.0 adds native Windows 11 support.

## Highlights

- Native Windows builds

## Install

```bash
curl -fsSL https://bast.sh/install | sh
```
EOF
)"
assert_eq "$expected" "$got" "cut preserves later version sections"

assert_fail "usage without args" bash "$script"
assert_fail "unknown command" bash "$script" frobnicate

if ! bash "$script" preview >/dev/null; then
  fail "preview should succeed when Unreleased is empty"
fi

got="$(CHANGELOG_PATH="$repo_root/CHANGELOG.md" bash "$script" extract v0.9.0)"
printf '%s\n' "$got" | grep -Fq "Native Windows AMD64 and ARM64 builds" || fail "repo changelog extract v0.9.0"
CHANGELOG_PATH="$repo_root/CHANGELOG.md" bash "$script" extract Unreleased >/dev/null || fail "repo Unreleased should be non-empty"

echo "changelog.sh tests passed"
