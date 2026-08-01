#!/usr/bin/env bash
# Copy the canonical agent skill into the website public dir and refresh its checksum.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
src="$root/skills/bast/SKILL.md"
dest="$root/apps/web/public/bast.skill.md"
checksum="$root/apps/web/public/bast.skill.md.sha256"

if [[ ! -f "$src" ]]; then
  echo "sync-skill: missing $src" >&2
  exit 1
fi

cp "$src" "$dest"

if command -v shasum >/dev/null 2>&1; then
  (cd "$(dirname "$dest")" && shasum -a 256 "$(basename "$dest")" >"$(basename "$checksum")")
elif command -v sha256sum >/dev/null 2>&1; then
  (cd "$(dirname "$dest")" && sha256sum "$(basename "$dest")" >"$(basename "$checksum")")
else
  echo "sync-skill: shasum or sha256sum is required" >&2
  exit 1
fi

echo "Synced $src -> $dest (+ checksum)"
