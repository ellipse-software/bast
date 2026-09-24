#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT
mkdir -p "$test_root/bin"

cat > "$test_root/bin/gh" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "$GH_CALLS"
case "$1 $2" in
  'release view')
    [[ "$SCENARIO" != missing-release ]] || exit 1
    jq -n --arg scenario "$SCENARIO" '{tagName: "v0.10.2", isDraft: ($scenario == "draft"), isPrerelease: ($scenario == "prerelease")}'
    ;;
  'api repos/microsoft/winget-pkgs/contents/manifests/e/EllipseSoftware/Bast')
    [[ "$SCENARIO" != api-failure ]] || exit 1
    if [[ "$SCENARIO" == merged ]]; then
      printf '[{"name":"0.10.2"}]\n'
    else
      printf '[{"name":"0.9.0"}]\n'
    fi
    ;;
  'pr list')
    [[ "$*" == *'--state open'* ]] || exit 1
    [[ "$SCENARIO" != pr-failure ]] || exit 1
    case "$SCENARIO" in
      open-pr) printf '[{"title":"New version: EllipseSoftware.Bast version 0.10.2","url":"https://github.com/microsoft/winget-pkgs/pull/123"}]\n' ;;
      other-version) printf '[{"title":"New version: EllipseSoftware.Bast version 0.10.20","url":"https://github.com/microsoft/winget-pkgs/pull/456"}]\n' ;;
      *) printf '[]\n' ;;
    esac
    ;;
  'release download')
    [[ "$SCENARIO" != missing-asset ]] || exit 1
    filename="$5"
    output_dir="$7"
    [[ "$SCENARIO" != wrong-package ]] && package=EllipseSoftware.Bast || package=Other.Package
    [[ "$SCENARIO" != wrong-version ]] && version=0.10.2 || version=0.9.0
    printf 'PackageIdentifier: %s\nPackageVersion: %s\n' "$package" "$version" > "$output_dir/$filename"
    ;;
  *) echo "Unexpected gh command: $*" >&2; exit 1 ;;
esac
MOCK
chmod +x "$test_root/bin/gh"
export PATH="$test_root/bin:$PATH"
export GH_CALLS="$test_root/calls"
export GITHUB_OUTPUT="$test_root/outputs"

run_case() {
  export SCENARIO="$1"
  : > "$GH_CALLS"
  : > "$GITHUB_OUTPUT"
  bash "$repo_root/.github/scripts/prepare-winget-submission.sh" "${2:-v0.10.2}" "$test_root/$SCENARIO"
}

for scenario in new other-version; do
  run_case "$scenario"
  grep -Fxq 'submit=true' "$GITHUB_OUTPUT"
  test "$(find "$test_root/$scenario" -name '*.yaml' | wc -l | tr -d ' ')" = 3
done

for scenario in merged open-pr; do
  run_case "$scenario"
  grep -Fxq 'submit=false' "$GITHUB_OUTPUT"
  if grep -q 'release download' "$GH_CALLS"; then
    echo "FAIL: $scenario downloaded manifests despite an existing submission" >&2
    exit 1
  fi
done

for scenario in missing-release draft prerelease api-failure pr-failure missing-asset wrong-package wrong-version; do
  if run_case "$scenario" > "$test_root/error" 2>&1; then
    echo "FAIL: $scenario should fail" >&2
    exit 1
  fi
  if grep -Fxq 'submit=true' "$GITHUB_OUTPUT"; then
    echo "FAIL: $scenario enabled submission" >&2
    exit 1
  fi
done

for tag in nightly v0.10.2-rc.1 0.10.2 'v0.10.2;echo invalid'; do
  if run_case invalid-tag "$tag" > "$test_root/error" 2>&1; then
    echo "FAIL: accepted invalid tag $tag" >&2
    exit 1
  fi
  test ! -s "$GH_CALLS"
done

echo 'WinGet submission preparation tests passed.'
