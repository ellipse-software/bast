#!/usr/bin/env bash
set -euo pipefail

version="${1:?usage: bump-homebrew-tap-nightly.sh <version>}"
tag="$version"

bast_repo="ellipse-software/bast"
tap_repo="ellipse-software/homebrew-tap"
tap_dir="$(mktemp -d "${TMPDIR:-/tmp}/homebrew-tap.XXXXXX")"

cleanup() {
  rm -rf "$tap_dir"
}
trap cleanup EXIT

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "GH_TOKEN is required" >&2
  exit 1
fi

if [[ "$version" != nightly.* ]]; then
  echo "version must start with nightly. (got: $version)" >&2
  exit 1
fi

sums_url="https://github.com/${bast_repo}/releases/download/${tag}/SHA256SUMS"
sums_file="$(mktemp)"
curl -fsSL "$sums_url" -o "$sums_file"

declare -A checksums=()
while read -r hash archive; do
  [[ -n "$hash" && -n "$archive" ]] || continue
  checksums["$archive"]="$hash"
done < "$sums_file"

required=(
  "bast_${version}_darwin_amd64.tar.gz"
  "bast_${version}_darwin_arm64.tar.gz"
  "bast_${version}_linux_amd64.tar.gz"
  "bast_${version}_linux_arm64.tar.gz"
)

for archive in "${required[@]}"; do
  if [[ -z "${checksums[$archive]:-}" ]]; then
    echo "missing checksum for ${archive} in ${sums_url}" >&2
    exit 1
  fi
done

git clone --depth 1 "https://x-access-token:${GH_TOKEN}@github.com/${tap_repo}.git" "$tap_dir"

cat > "${tap_dir}/Formula/bast-nightly.rb" <<EOF
class BastNightly < Formula
  desc "Browse SSH hosts, manage keys, and connect from the terminal (nightly)"
  homepage "https://bast.sh"
  version "${version}"
  license "MIT"

  conflicts_with "bast"

  on_macos do
    on_arm do
      url "https://github.com/${bast_repo}/releases/download/${tag}/bast_${version}_darwin_arm64.tar.gz"
      sha256 "${checksums[bast_${version}_darwin_arm64.tar.gz]}"
    end
    on_intel do
      url "https://github.com/${bast_repo}/releases/download/${tag}/bast_${version}_darwin_amd64.tar.gz"
      sha256 "${checksums[bast_${version}_darwin_amd64.tar.gz]}"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/${bast_repo}/releases/download/${tag}/bast_${version}_linux_arm64.tar.gz"
      sha256 "${checksums[bast_${version}_linux_arm64.tar.gz]}"
    end
    on_intel do
      url "https://github.com/${bast_repo}/releases/download/${tag}/bast_${version}_linux_amd64.tar.gz"
      sha256 "${checksums[bast_${version}_linux_amd64.tar.gz]}"
    end
  end

  def install
    bin.install "bast"
  end

  test do
    assert_match "bast #{version}", shell_output("#{bin}/bast --version")
  end
end
EOF

pushd "$tap_dir" >/dev/null
git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

if git diff --quiet; then
  echo "Formula already up to date for ${version}"
  exit 0
fi

git add Formula/bast-nightly.rb
git commit -m "bast-nightly ${version}"
git push origin HEAD
popd >/dev/null

echo "Updated ${tap_repo} bast-nightly to ${version}"
