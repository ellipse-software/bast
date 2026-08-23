#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
changelog="${CHANGELOG_PATH:-$repo_root/CHANGELOG.md}"

usage() {
  cat <<'EOF' >&2
usage: changelog.sh extract <tag>
       changelog.sh cut <tag>
       changelog.sh preview
EOF
}

heading_ref() {
  local line="$1"
  if [[ "$line" =~ ^##[[:space:]]+Unreleased[[:space:]]*$ ]]; then
    printf '%s\n' "Unreleased"
    return 0
  fi
  if [[ "$line" =~ ^##[[:space:]]+(v[0-9][^[:space:]]*)([[:space:]]+-[[:space:]]+[0-9]{4}-[0-9]{2}-[0-9]{2})?[[:space:]]*$ ]]; then
    printf '%s\n' "${BASH_REMATCH[1]}"
    return 0
  fi
  return 1
}

trim_section() {
  local raw="$1"
  local -a lines=()
  local line start=-1 end=-1 i n

  while IFS= read -r line || [[ -n "$line" ]]; do
    lines+=("$line")
  done < <(printf '%s' "$raw")

  n=${#lines[@]}
  i=0
  while [[ $i -lt $n ]]; do
    if [[ ! "${lines[$i]}" =~ ^[[:space:]]*$ ]]; then
      if [[ $start -eq -1 ]]; then
        start=$i
      fi
      end=$i
    fi
    i=$((i + 1))
  done

  if [[ $start -eq -1 ]]; then
    return 1
  fi

  i=$start
  while [[ $i -le $end ]]; do
    printf '%s\n' "${lines[$i]}"
    i=$((i + 1))
  done
}

require_changelog() {
  if [[ ! -f "$changelog" ]]; then
    echo "changelog not found: $changelog" >&2
    exit 1
  fi
}

extract_section() {
  local target="$1"
  local in_section=0
  local found=0
  local body=""
  local line ref

  require_changelog

  while IFS= read -r line || [[ -n "$line" ]]; do
    if ref="$(heading_ref "$line")"; then
      if [[ "$in_section" -eq 1 ]]; then
        break
      fi
      if [[ "$ref" == "$target" ]]; then
        in_section=1
        found=1
        continue
      fi
    fi
    if [[ "$in_section" -eq 1 ]]; then
      body+="$line"$'\n'
    fi
  done < "$changelog"

  if [[ "$found" -eq 0 ]]; then
    echo "no changelog section for ${target}" >&2
    return 1
  fi

  local trimmed
  if ! trimmed="$(trim_section "$body")"; then
    echo "changelog section ${target} is empty" >&2
    return 1
  fi
  printf '%s\n' "$trimmed"
}

validate_tag() {
  local tag="$1"
  if [[ ! "$tag" =~ ^v[0-9][^[:space:]]*$ ]]; then
    echo "tag must match v* with a leading digit (got: ${tag})" >&2
    exit 1
  fi
}

section_exists() {
  local target="$1"
  local line ref
  require_changelog
  while IFS= read -r line || [[ -n "$line" ]]; do
    if ref="$(heading_ref "$line")" && [[ "$ref" == "$target" ]]; then
      return 0
    fi
  done < "$changelog"
  return 1
}

cut_release() {
  local tag="$1"
  local line ref
  local phase=prefix
  local prefix=""
  local unreleased_body=""
  local suffix=""
  local date
  local trimmed
  local tmp

  validate_tag "$tag"
  require_changelog

  if section_exists "$tag"; then
    echo "changelog already has ${tag}" >&2
    exit 1
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    if ref="$(heading_ref "$line")"; then
      if [[ "$phase" == prefix && "$ref" == Unreleased ]]; then
        phase=unreleased
        continue
      fi
      if [[ "$phase" == unreleased ]]; then
        phase=suffix
      fi
    fi
    case "$phase" in
      prefix) prefix+="$line"$'\n' ;;
      unreleased) unreleased_body+="$line"$'\n' ;;
      suffix) suffix+="$line"$'\n' ;;
    esac
  done < "$changelog"

  if [[ "$phase" == prefix ]]; then
    echo "no Unreleased section in $changelog" >&2
    exit 1
  fi

  if ! trimmed="$(trim_section "$unreleased_body")"; then
    echo "Unreleased changelog is empty" >&2
    exit 1
  fi

  date="${CHANGELOG_DATE:-$(date -u +%Y-%m-%d)}"
  tmp="${changelog}.tmp.$$"

  {
    printf '%s' "$prefix"
    if [[ -n "$prefix" && "$prefix" != *$'\n' ]]; then
      printf '\n'
    fi
    printf '%s\n' "## Unreleased"
    printf '\n'
    printf '%s\n' "## ${tag} - ${date}"
    printf '\n'
    printf '%s\n' "$trimmed"
    if [[ -n "$suffix" ]]; then
      printf '\n'
      printf '%s' "$suffix"
      if [[ "$suffix" != *$'\n' ]]; then
        printf '\n'
      fi
    fi
  } > "$tmp"

  mv "$tmp" "$changelog"
}

latest_stable_tag() {
  local tag
  while IFS= read -r tag; do
    [[ -n "$tag" ]] || continue
    if [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      printf '%s\n' "$tag"
      return 0
    fi
  done < <(git -C "$repo_root" tag --list 'v*' --sort=-version:refname)
  return 1
}

preview() {
  local latest=""
  local log_range="HEAD"

  if git -C "$repo_root" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    if latest="$(latest_stable_tag)"; then
      if git -C "$repo_root" rev-parse --verify "$latest" >/dev/null 2>&1; then
        log_range="${latest}..HEAD"
      fi
    fi
    printf 'Commits since %s:\n\n' "${latest:-initial commit}"
    git -C "$repo_root" --no-pager log --oneline "$log_range" || true
    printf '\n'
  fi

  printf 'Unreleased:\n\n'
  if unreleased="$(extract_section "Unreleased" 2>/dev/null)"; then
    printf '%s\n' "$unreleased"
  else
    printf '%s\n' "(empty)"
  fi
}

if [[ $# -lt 1 ]]; then
  usage
  exit 2
fi

cmd="$1"
shift

case "$cmd" in
  extract)
    if [[ $# -ne 1 ]]; then
      usage
      exit 2
    fi
    extract_section "$1"
    ;;
  cut)
    if [[ $# -ne 1 ]]; then
      usage
      exit 2
    fi
    cut_release "$1"
    ;;
  preview)
    if [[ $# -ne 0 ]]; then
      usage
      exit 2
    fi
    preview
    ;;
  *)
    usage
    exit 2
    ;;
esac
