#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
script="$repo_root/.github/scripts/package-linux.sh"
test_root="$(mktemp -d)"

cleanup() {
  rm -rf "$test_root"
}
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_fail() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    fail "$label: expected failure"
  fi
}

cd "$repo_root"

bash -n "$script"
bash -n "$repo_root/.github/scripts/publish-linux-repo.sh"
sh -n "$repo_root/.github/packaging/setup.sh"
sh -n "$repo_root/apps/web/public/install"
sh -n "$repo_root/apps/web/public/install-nightly"

assert_fail "rejects missing tag" bash "$script"
assert_fail "rejects nightly tag" bash "$script" "nightly.20260823.abc1234" "$test_root/dist"
assert_fail "rejects prerelease tag" bash "$script" "v1.2.3-beta" "$test_root/dist"
assert_fail "rejects missing dist dir" bash "$script" "v1.2.3" "$test_root/missing"

mkdir -p "$test_root/empty"
assert_fail "rejects missing linux archives" bash "$script" "v1.2.3" "$test_root/empty"

dist_dir="$test_root/dist"
mkdir -p "$dist_dir"

make_archive() {
  local goarch="$1"
  local bundle="bast_1.2.3_linux_${goarch}"
  local stage="$test_root/stage/$bundle"
  mkdir -p "$stage"
  printf '#!/bin/sh\necho bast v1.2.3\n' >"$stage/bast"
  chmod +x "$stage/bast"
  cp "$repo_root/LICENSE" "$repo_root/apps/bast/README.md" "$stage/"
  tar -C "$test_root/stage" -czf "$dist_dir/$bundle.tar.gz" "$bundle"
  if command -v shasum >/dev/null 2>&1; then
    (cd "$dist_dir" && shasum -a 256 "$bundle.tar.gz" >"$bundle.tar.gz.sha256")
  else
    (cd "$dist_dir" && sha256sum "$bundle.tar.gz" >"$bundle.tar.gz.sha256")
  fi
}

make_archive amd64
assert_fail "rejects incomplete arch set" bash "$script" "v1.2.3" "$dist_dir"
make_archive arm64

if [[ "${PACKAGE_LINUX_SKIP_NFPM:-}" == "1" ]]; then
  echo "skipping nFPM packaging (PACKAGE_LINUX_SKIP_NFPM=1)"
  exit 0
fi

bash "$script" "v1.2.3" "$dist_dir"

for goarch in amd64 arm64; do
  for ext in deb rpm apk pkg.tar.zst; do
    file="$dist_dir/bast_1.2.3_linux_${goarch}.${ext}"
    [[ -f "$file" ]] || fail "missing $file"
    [[ -f "$file.sha256" ]] || fail "missing $file.sha256"
    if command -v shasum >/dev/null 2>&1; then
      (cd "$dist_dir" && shasum -a 256 -c "$(basename "$file").sha256") >/dev/null
    else
      (cd "$dist_dir" && sha256sum -c "$(basename "$file").sha256") >/dev/null
    fi
  done
done

if command -v dpkg-deb >/dev/null 2>&1; then
  pkg_name="$(dpkg-deb -f "$dist_dir/bast_1.2.3_linux_amd64.deb" Package)"
  [[ "$pkg_name" == "bast" ]] || fail "deb package name is $pkg_name"
  depends="$(dpkg-deb -f "$dist_dir/bast_1.2.3_linux_amd64.deb" Depends)"
  [[ "$depends" == *openssh-client* ]] || fail "deb depends missing openssh-client: $depends"
fi

if command -v rpm >/dev/null 2>&1; then
  rpm_name="$(rpm -qp --qf '%{NAME}' "$dist_dir/bast_1.2.3_linux_amd64.rpm")"
  [[ "$rpm_name" == "bast" ]] || fail "rpm package name is $rpm_name"
fi

echo "package-linux.test.sh ok"
