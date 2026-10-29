#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
script="$repo_root/.github/scripts/discord-release.sh"
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

unset BAST_REPO DISCORD_RELEASE_CONTENT DISCORD_RELEASES_WEBHOOK_URL || true

changelog="$test_root/CHANGELOG.md"
export CHANGELOG_PATH="$changelog"

write_fixture() {
  cat > "$changelog" <<'EOF'
# Changelog

## Unreleased

### :)

## v0.11.0 - 2026-10-27

### Added

- Open Remote Desktop.

```bash
echo "## not a heading"
```

### Changes

- Host forms save with Ctrl+S.

### Fixed

- Sync tab icons.

Get started at [https://bast.sh](https://bast.sh) or update with `bast update`.

## v0.10.4 - 2026-10-03

- Plain notes.

Get started at [https://bast.sh](https://bast.sh) or update with `bast update`.
EOF
}

embed_size() {
  jq -r '
    [.embeds[]
      | ((.title // "") | length)
        + ((.description // "") | length)
        + ((.footer.text // "") | length)
    ] | add // 0
  '
}

assert_within_limits() {
  local payload="$1"
  local label="$2"
  local size too_long embed_count

  size="$(printf '%s' "$payload" | embed_size)"
  if (( size > 6000 )); then
    fail "$label: embed text is $size characters"
  fi
  too_long="$(printf '%s' "$payload" | jq '[.embeds[] | select(((.description // "") | length) > 4096)] | length')"
  if [[ "$too_long" != "0" ]]; then
    fail "$label: a description exceeds 4096 characters"
  fi
  embed_count="$(printf '%s' "$payload" | jq '.embeds | length')"
  if (( embed_count > 10 || embed_count < 1 )); then
    fail "$label: $embed_count embeds"
  fi
}

write_fixture

got="$(bash "$script" payload v0.11.0)"
assert_within_limits "$got" "fixture v0.11.0"
assert_eq "1" "$(printf '%s' "$got" | jq '.embeds | length')" "fixture is one embed"
if printf '%s' "$got" | jq -e '.embeds[0] | has("fields")' >/dev/null; then
  fail "release notes were split into fields"
fi
if ! printf '%s' "$got" | jq -e '.embeds[0].description | contains("**Added**") and contains("Open Remote Desktop.") and contains("**Changes**") and contains("**Fixed**")' >/dev/null; then
  fail "fixture description dropped a section"
fi
assert_eq "Bast v0.11.0" "$(printf '%s' "$got" | jq -r '.embeds[0].title')" "title"
assert_eq "https://github.com/ellipse-software/bast/releases/tag/v0.11.0" \
  "$(printf '%s' "$got" | jq -r '.embeds[0].url')" "release url"
assert_eq "9133302" "$(printf '%s' "$got" | jq -r '.embeds[0].color')" "accent color"
assert_eq "Bast" "$(printf '%s' "$got" | jq -r '.username')" "username"
assert_eq "[]" "$(printf '%s' "$got" | jq -c '.allowed_mentions.parse')" "mentions disabled"
assert_eq "null" "$(printf '%s' "$got" | jq -r '.content')" "no content by default"
assert_eq '[{"type":2,"style":5,"label":"GitHub","url":"https://github.com/ellipse-software/bast/releases/tag/v0.11.0"},{"type":2,"style":5,"label":"Changelog","url":"https://bast.sh/changelog"},{"type":2,"style":5,"label":"Website","url":"https://bast.sh"}]' \
  "$(printf '%s' "$got" | jq -c '.components[0].components')" "link buttons"
if ! printf '%s' "$got" | jq -e '.embeds[0].description | contains("## not a heading")' >/dev/null; then
  fail "code fence heading was split into its own section"
fi
if printf '%s' "$got" | jq -e '.embeds[0].description | contains("Get started at")' >/dev/null; then
  fail "get-started line stayed in the embed body"
fi
footer="$(printf '%s' "$got" | jq -r '.embeds[0].footer.text')"
assert_eq "2026-10-27 · Get started at bast.sh or update with bast update" "$footer" "footer"

got="$(bash "$script" payload v0.10.4)"
assert_within_limits "$got" "plain notes"
assert_eq "- Plain notes." "$(printf '%s' "$got" | jq -r '.embeds[0].description')" "plain description"
assert_eq "1" "$(printf '%s' "$got" | jq '.embeds | length')" "plain notes stay one embed"
assert_eq "2026-10-03 · Get started at bast.sh or update with bast update" \
  "$(printf '%s' "$got" | jq -r '.embeds[0].footer.text')" "plain footer"

long_line="- $(printf 'x%.0s' {1..1100})"
cat > "$changelog" <<EOF
# Changelog

## v1.2.3 - 2026-02-02

### Added

$long_line
EOF
got="$(bash "$script" payload v1.2.3)"
assert_within_limits "$got" "long bullet"
assert_eq "1" "$(printf '%s' "$got" | jq '.embeds | length')" "long bullet stays one embed"
x_count="$(printf '%s' "$got" | jq -r '[.embeds[].description] | join("")' | tr -cd 'x' | wc -c | tr -d ' ')"
assert_eq "1100" "$x_count" "long bullet keeps every character"

{
  echo "# Changelog"
  echo
  echo "## v9.9.9 - 2026-01-01"
  echo
  echo "### Added"
  echo
  printf -- '- '
  printf 'y%.0s' {1..5000}
  printf '\n'
} > "$changelog"
got="$(bash "$script" payload v9.9.9)"
assert_within_limits "$got" "section over one description"
assert_eq "2" "$(printf '%s' "$got" | jq '.embeds | length')" "long section uses a second embed"
y_count="$(printf '%s' "$got" | jq -r '[.embeds[].description] | join("")' | tr -cd 'y' | wc -c | tr -d ' ')"
assert_eq "5000" "$y_count" "long section keeps every character"
assert_eq "Bast v9.9.9" "$(printf '%s' "$got" | jq -r '.embeds[0].title')" "title stays on the first embed"
assert_eq "null" "$(printf '%s' "$got" | jq -r '.embeds[1].title')" "continuation has no title"
assert_eq "2026-01-01" "$(printf '%s' "$got" | jq -r '.embeds[-1].footer.text')" "footer stays on the last embed"

{
  echo "# Changelog"
  echo
  echo "## v8.0.0 - 2026-01-02"
  echo
  echo "### Added"
  echo
  i=1
  while (( i <= 120 )); do
    printf -- '- item %03d ' "$i"
    printf 'y%.0s' {1..70}
    printf '\n'
    i=$((i + 1))
  done
} > "$changelog"
got="$(bash "$script" payload v8.0.0)"
assert_within_limits "$got" "oversized section"
if ! printf '%s' "$got" | jq -e '.embeds[0].description | contains("item 001")' >/dev/null; then
  fail "oversized section lost its first bullet"
fi
if ! printf '%s' "$got" | jq -e '.embeds[-1].description | endswith("…")' >/dev/null; then
  fail "oversized section was not marked as truncated"
fi

write_fixture
got="$(DISCORD_RELEASE_CONTENT='Trial line' BAST_REPO='example/bast' bash "$script" payload v0.11.0)"
assert_eq "Trial line" "$(printf '%s' "$got" | jq -r '.content')" "trial content"
assert_eq "https://github.com/example/bast/releases/tag/v0.11.0" \
  "$(printf '%s' "$got" | jq -r '.embeds[0].url')" "repo override"
assert_eq "https://github.com/example/bast/releases/tag/v0.11.0" \
  "$(printf '%s' "$got" | jq -r '.components[0].components[0].url')" "github button follows the repo"

write_fixture
assert_fail "reject a non-repository override" env BAST_REPO='https://evil.example' bash "$script" payload v0.11.0
assert_fail "unknown command" bash "$script" payload-not-post
assert_fail "post without a webhook" env DISCORD_RELEASES_WEBHOOK_URL= bash "$script" post v0.11.0
assert_fail "reject a non-discord webhook" env DISCORD_RELEASES_WEBHOOK_URL='https://example.com/hook' bash "$script" post v0.11.0
assert_fail "usage without args" bash "$script"

repo_payload="$(CHANGELOG_PATH="$repo_root/CHANGELOG.md" bash "$script" payload v0.11.0)"
assert_within_limits "$repo_payload" "repo v0.11.0"
assert_eq "Bast v0.11.0" "$(printf '%s' "$repo_payload" | jq -r '.embeds[0].title')" "repo title"
assert_eq "2026-10-27 · Get started at bast.sh or update with bast update" \
  "$(printf '%s' "$repo_payload" | jq -r '.embeds[0].footer.text')" "repo footer"
assert_eq "1" "$(printf '%s' "$repo_payload" | jq '.embeds | length')" "repo v0.11.0 is one embed"
for needle in "**Added**" "**Changes**" "**Fixed**" "BitQate" "Railway" "Fly.io" "DigitalOcean" "Ctrl+S" "Nerd Font"; do
  if ! printf '%s' "$repo_payload" | jq -e --arg needle "$needle" '[.embeds[].description] | join("\n") | contains($needle)' >/dev/null; then
    fail "repo v0.11.0 missing $needle"
  fi
done
if printf '%s' "$repo_payload" | jq -e '[.embeds[].description] | join("\n") | contains("Get started at")' >/dev/null; then
  fail "repo get-started line stayed in the body"
fi

echo "discord-release.sh tests passed"
