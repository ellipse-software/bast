#!/usr/bin/env bash
set -euo pipefail

tag="${1:?release tag is required}"
output_dir="${2:?manifest output directory is required}"
version="${tag#v}"
package_id="EllipseSoftware.Bast"

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "WinGet submissions require a stable vX.Y.Z tag" >&2
  exit 1
fi

release="$(gh release view "$tag" --json tagName,isDraft,isPrerelease)"
if ! jq -e --arg tag "$tag" '.tagName == $tag and .isDraft == false and .isPrerelease == false' <<< "$release" >/dev/null; then
  echo "Release $tag must be published and stable before submitting to WinGet" >&2
  exit 1
fi

# Read the existing package directory so API failures cannot look like an absent version.
versions="$(gh api repos/microsoft/winget-pkgs/contents/manifests/e/EllipseSoftware/Bast)"
if jq -e --arg version "$version" 'any(.[]; .name == $version)' <<< "$versions" >/dev/null; then
  echo "$package_id $version is already in WinGet."
  echo 'submit=false' >> "${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}"
  exit 0
fi

pull_requests="$(gh pr list --repo microsoft/winget-pkgs --state open \
  --search "\"$package_id\" \"$version\" in:title" --limit 100 --json title,url)"
existing_pr="$(jq -r --arg package "$package_id" --arg version "$version" \
  '[.[] | select(.title | contains($package)) | select(.title | test("(^|[^0-9.])" + ($version | gsub("\\."; "\\.")) + "([^0-9.]|$)"))][0].url // empty' \
  <<< "$pull_requests")"
if [[ -n "$existing_pr" ]]; then
  echo "WinGet submission already open: $existing_pr"
  echo 'submit=false' >> "$GITHUB_OUTPUT"
  exit 0
fi

# Submit the exact manifests attached to the release, including its signed archive hashes.
mkdir -p "$output_dir"
for filename in "$package_id.yaml" "$package_id.installer.yaml" "$package_id.locale.en-US.yaml"; do
  gh release download "$tag" --pattern "$filename" --dir "$output_dir"
  if ! grep -Fxq "PackageIdentifier: $package_id" "$output_dir/$filename" ||
     ! grep -Fxq "PackageVersion: $version" "$output_dir/$filename"; then
    echo "Unexpected package identifier or version in $filename" >&2
    exit 1
  fi
done

echo 'submit=true' >> "$GITHUB_OUTPUT"
