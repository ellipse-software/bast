#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

usage() {
  cat <<'EOF' >&2
usage: discord-release.sh payload <tag>
       discord-release.sh post <tag>

payload writes the Discord webhook JSON for a changelog section.
post sends it to DISCORD_RELEASES_WEBHOOK_URL.

BAST_REPO overrides the GitHub repository (default ellipse-software/bast).
DISCORD_RELEASE_CONTENT sets an optional message above the embed.
CHANGELOG_PATH overrides the changelog file.
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "required command not found: $1" >&2
    exit 1
  fi
}

changelog_date() {
  local tag="$1"
  local changelog="${CHANGELOG_PATH:-$repo_root/CHANGELOG.md}"
  local line rest

  if [[ ! -f "$changelog" ]]; then
    echo "changelog not found: $changelog" >&2
    exit 1
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    rest="${line#"## ${tag} - "}"
    if [[ "$rest" == "$line" ]]; then
      continue
    fi
    rest="${rest#"${rest%%[![:space:]]*}"}"
    rest="${rest%"${rest##*[![:space:]]}"}"
    if [[ "$rest" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
      printf '%s\n' "$rest"
      return 0
    fi
  done < "$changelog"
}

payload() {
  local tag="$1"
  local notes date repo content
  local -a args

  require_cmd python3
  notes="$(bash "$script_dir/changelog.sh" extract "$tag")"
  date="$(changelog_date "$tag")"
  repo="${BAST_REPO:-ellipse-software/bast}"
  content="${DISCORD_RELEASE_CONTENT:-}"

  args=(python3 "$script_dir/discord-release.py" --tag "$tag" --repo "$repo")
  if [[ -n "$date" ]]; then
    args+=(--date "$date")
  fi
  if [[ -n "$content" ]]; then
    args+=(--content "$content")
  fi
  printf '%s\n' "$notes" | "${args[@]}"
}

redact() {
  sed -E 's#https://(discord\.com|discordapp\.com)/api/webhooks/[0-9]+/[^[:space:]"]+#https://discord.com/api/webhooks/[redacted]#g'
}

post_once() {
  local url="$1"
  local payload_file="$2"
  local body="$3"
  local err="$4"

  curl -sS -o "$body" -w "%{http_code}" --max-time 30 \
    -H "Content-Type: application/json; charset=utf-8" \
    -X POST \
    --data-binary @"$payload_file" \
    "${url}?wait=true&with_components=true" 2>"$err" || true
}

clamp_retry() {
  python3 -c 'import sys
try:
    value = float(sys.argv[1])
except ValueError:
    value = 1
print(min(10, max(0, value)))' "$1"
}

post() {
  local tag="$1"
  local url="${DISCORD_RELEASES_WEBHOOK_URL:-}"
  local payload_file body err code wait_s message_id

  if [[ -z "$url" ]]; then
    echo "DISCORD_RELEASES_WEBHOOK_URL is not set" >&2
    exit 1
  fi
  if [[ ! "$url" =~ ^https://(discord\.com|discordapp\.com)/api/webhooks/[0-9]+/[^[:space:]]+$ ]]; then
    echo "DISCORD_RELEASES_WEBHOOK_URL is not a Discord incoming webhook URL" >&2
    exit 1
  fi

  require_cmd curl
  require_cmd jq
  payload_file="$(mktemp)"
  body="$(mktemp)"
  err="$(mktemp)"
  trap 'rm -f "$payload_file" "$body" "$err"' RETURN

  payload "$tag" > "$payload_file"
  code="$(post_once "$url" "$payload_file" "$body" "$err")"
  if [[ "$code" == "429" ]]; then
    wait_s="$(jq -r '.retry_after // 1' "$body" 2>/dev/null || echo 1)"
    sleep "$(clamp_retry "$wait_s")"
    code="$(post_once "$url" "$payload_file" "$body" "$err")"
  fi
  if [[ "$code" != "200" ]]; then
    echo "Discord rejected the release embed (HTTP ${code:-failed})" >&2
    if [[ -s "$err" ]]; then
      redact < "$err" >&2
    fi
    if [[ -s "$body" ]]; then
      redact < "$body" >&2
    fi
    exit 1
  fi

  message_id="$(jq -r '.id // empty' "$body")"
  if [[ -z "$message_id" ]]; then
    echo "Discord accepted the embed but returned no message id" >&2
    exit 1
  fi
  printf '%s\n' "$message_id"
}

if [[ $# -lt 1 ]]; then
  usage
  exit 2
fi

cmd="$1"
shift

case "$cmd" in
  payload)
    if [[ $# -ne 1 ]]; then
      usage
      exit 2
    fi
    payload "$1"
    ;;
  post)
    if [[ $# -ne 1 ]]; then
      usage
      exit 2
    fi
    post "$1"
    ;;
  *)
    usage
    exit 2
    ;;
esac
