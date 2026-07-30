#!/bin/sh
# Install the Bast agent skill for Cursor, Claude Code, and Codex.
set -eu

SKILL_URL="${BAST_SKILL_URL:-https://bast.sh/bast.skill.md}"
CHECKSUM_URL="${BAST_SKILL_CHECKSUM_URL:-$SKILL_URL.sha256}"
SCOPE="${BAST_SKILL_SCOPE:-user}"

case "$SCOPE" in
  user) ;;
  project) ;;
  *)
    echo "bast install-skill: BAST_SKILL_SCOPE must be 'user' or 'project'" >&2
    exit 2
    ;;
esac

temp_dir=$(mktemp -d)
tmp="$temp_dir/bast.skill.md"
checksum="$temp_dir/bast.skill.md.sha256"
trap 'rm -rf "$temp_dir"' EXIT INT TERM

curl -fsSL "$SKILL_URL" -o "$tmp"
curl -fsSL "$CHECKSUM_URL" -o "$checksum"

if command -v shasum >/dev/null 2>&1; then
  (cd "$temp_dir" && shasum -a 256 -c "bast.skill.md.sha256") >/dev/null
elif command -v sha256sum >/dev/null 2>&1; then
  (cd "$temp_dir" && sha256sum -c "bast.skill.md.sha256") >/dev/null
else
  echo "bast install-skill: shasum or sha256sum is required" >&2
  exit 1
fi

install_skill() {
  agent="$1"
  base="$2"
  dir="$base/bast"
  mkdir -p "$dir"
  cp "$tmp" "$dir/SKILL.md"
  printf '  %s → %s/SKILL.md\n' "$agent" "$dir"
}

if [ "$SCOPE" = "project" ]; then
  root="${BAST_SKILL_PROJECT_ROOT:-.}"
  echo "Installing Bast skill for this project ($root):"
  install_skill "Cursor" "$root/.cursor/skills"
  install_skill "Claude Code" "$root/.claude/skills"
  install_skill "Codex" "$root/.codex/skills"
else
  echo "Installing Bast skill for your user account:"
  install_skill "Cursor" "${CURSOR_SKILLS:-$HOME/.cursor/skills}"
  install_skill "Claude Code" "${CLAUDE_SKILLS:-$HOME/.claude/skills}"
  install_skill "Codex" "${CODEX_HOME:-$HOME/.codex}/skills"
fi

echo "Done. Restart or start a new agent session to pick up the skill."
