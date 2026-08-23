#!/usr/bin/env bash
# Build signed apt/rpm/apk/arch repositories from package-linux.sh output
# and upload them to the bast-packages R2 bucket.
#
# Required environment:
#   LINUX_SIGNING_PRIVATE_KEY  ASCII-armored or base64 OpenPGP private key
#   LINUX_SIGNING_PUBLIC_KEY   ASCII-armored or base64 OpenPGP public key
#   LINUX_SIGNING_PASSPHRASE   passphrase (empty if the key is unprotected)
#   PACKAGES_R2_ACCOUNT_ID
#   PACKAGES_R2_ACCESS_KEY_ID
#   PACKAGES_R2_SECRET_ACCESS_KEY
#   PACKAGES_R2_BUCKET         default bast-packages
#
# Optional:
#   LINUX_APK_RSA_PRIVATE / LINUX_APK_RSA_PUBLIC  PEM RSA keypair for Alpine
#   BAST_PACKAGES_URL                              default https://packages.bast.sh
set -euo pipefail

tag="${1:?usage: publish-linux-repo.sh <tag> <dist_dir>}"
dist_dir="${2:-dist}"
version="${tag#v}"
PACKAGES_R2_BUCKET="${PACKAGES_R2_BUCKET:-bast-packages}"
BAST_PACKAGES_URL="${BAST_PACKAGES_URL:-https://packages.bast.sh}"

if [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Linux repos require a stable vX.Y.Z tag (got: $tag)" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "$name is required to publish the Linux package repository" >&2
    exit 1
  fi
}

require_env LINUX_SIGNING_PRIVATE_KEY
require_env LINUX_SIGNING_PUBLIC_KEY
require_env PACKAGES_R2_ACCOUNT_ID
require_env PACKAGES_R2_ACCESS_KEY_ID
require_env PACKAGES_R2_SECRET_ACCESS_KEY

# GitHub secrets often pick up a trailing newline from paste/`echo`.
# R2 Access Key IDs must be exactly 32 characters.
trim_secret() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

PACKAGES_R2_ACCOUNT_ID="$(trim_secret "$PACKAGES_R2_ACCOUNT_ID")"
PACKAGES_R2_ACCESS_KEY_ID="$(trim_secret "$PACKAGES_R2_ACCESS_KEY_ID")"
PACKAGES_R2_SECRET_ACCESS_KEY="$(trim_secret "$PACKAGES_R2_SECRET_ACCESS_KEY")"
PACKAGES_R2_BUCKET="$(trim_secret "$PACKAGES_R2_BUCKET")"

if [[ ${#PACKAGES_R2_ACCESS_KEY_ID} -ne 32 ]]; then
  echo "PACKAGES_R2_ACCESS_KEY_ID is ${#PACKAGES_R2_ACCESS_KEY_ID} characters; R2 access keys are 32. A trailing newline in the GitHub secret is the usual cause. Reset it with: printf '%s' 'KEY' | gh secret set PACKAGES_R2_ACCESS_KEY_ID" >&2
  exit 1
fi

for goarch in amd64 arm64; do
  for ext in deb rpm; do
    if [[ ! -f "$dist_dir/bast_${version}_linux_${goarch}.${ext}" ]]; then
      echo "missing $dist_dir/bast_${version}_linux_${goarch}.${ext}" >&2
      exit 1
    fi
  done
done

tmp_root="${TMPDIR:-/tmp}"
tmp_root="${tmp_root%/}"
work="$(mktemp -d "${tmp_root}/bast-repo.XXXXXX")"
gnupg_home="$work/gnupg"
repo="$work/repo"
mkdir -p "$gnupg_home" "$repo"
chmod 700 "$gnupg_home"

cleanup() {
  rm -rf "$work"
}
trap cleanup EXIT

export GNUPGHOME="$gnupg_home"
export GPG_TTY=""

cat >"$gnupg_home/gpg.conf" <<'EOF'
batch
pinentry-mode loopback
EOF
cat >"$gnupg_home/gpg-agent.conf" <<'EOF'
allow-loopback-pinentry
default-cache-ttl 3600
max-cache-ttl 3600
EOF

decode_key() {
  local value="$1"
  local dest="$2"
  if [[ "$value" == *"BEGIN PGP"* || "$value" == *"BEGIN RSA"* || "$value" == *"BEGIN OPENSSH"* ]]; then
    printf '%s\n' "$value" >"$dest"
    return 0
  fi
  printf '%s\n' "$value" | base64 -d >"$dest"
}

decode_key "$LINUX_SIGNING_PRIVATE_KEY" "$work/private.key"
decode_key "$LINUX_SIGNING_PUBLIC_KEY" "$work/public.key"

if [[ -n "${LINUX_SIGNING_PASSPHRASE:-}" ]]; then
  gpg --batch --yes --pinentry-mode loopback --passphrase "$LINUX_SIGNING_PASSPHRASE" --import "$work/private.key"
else
  gpg --batch --yes --import "$work/private.key"
fi
gpg --batch --yes --import "$work/public.key"

key_id="$(gpg --list-secret-keys --with-colons | awk -F: '/^sec:/ { print $5; exit }')"
if [[ -z "$key_id" ]]; then
  echo "failed to import the packaging GPG key" >&2
  exit 1
fi

gpg --batch --yes --export --armor "$key_id" >"$repo/bast.asc"
gpg --batch --yes --export "$key_id" >"$repo/bast.gpg"

passphrase_args=()
if [[ -n "${LINUX_SIGNING_PASSPHRASE:-}" ]]; then
  passphrase_args=(--pinentry-mode loopback --passphrase "$LINUX_SIGNING_PASSPHRASE")
  printf 'ok\n' | gpg --batch --yes "${passphrase_args[@]}" --clearsign >/dev/null
fi

sign_file() {
  local file="$1"
  gpg --batch --yes "${passphrase_args[@]}" --detach-sign --armor "$file"
}

# --- apt (reprepro) ---
if ! command -v reprepro >/dev/null 2>&1; then
  echo "reprepro is required to publish the apt repository" >&2
  exit 1
fi

apt_root="$work/apt"
mkdir -p "$apt_root/conf"
cat >"$apt_root/conf/distributions" <<EOF
Origin: bast
Label: bast
Codename: stable
Architectures: amd64 arm64
Components: main
Description: Bast apt repository
SignWith: $key_id
EOF
cat >"$apt_root/conf/options" <<'EOF'
verbose
basedir .
EOF

reprepro -b "$apt_root" includedeb stable \
  "$dist_dir/bast_${version}_linux_amd64.deb" \
  "$dist_dir/bast_${version}_linux_arm64.deb"

mkdir -p "$repo/apt"
cp -a "$apt_root/dists" "$apt_root/pool" "$repo/apt/"

# --- rpm (createrepo_c) ---
if ! command -v createrepo_c >/dev/null 2>&1 && ! command -v createrepo >/dev/null 2>&1; then
  echo "createrepo_c is required to publish the rpm repository" >&2
  exit 1
fi
createrepo_bin="$(command -v createrepo_c || command -v createrepo)"

rpm_home="$work/rpmhome"
mkdir -p "$rpm_home"
pass_file="$work/gpg-passphrase"
gpg_pass_flags="--pinentry-mode loopback"
if [[ -n "${LINUX_SIGNING_PASSPHRASE:-}" ]]; then
  printf '%s' "$LINUX_SIGNING_PASSPHRASE" >"$pass_file"
  chmod 600 "$pass_file"
  gpg_pass_flags="--pinentry-mode loopback --passphrase-file $pass_file"
fi
cat >"$rpm_home/.rpmmacros" <<EOF
%_gpg_name $key_id
%__gpg $(command -v gpg)
%__gpg_sign_cmd %{__gpg} gpg --batch --yes --no-armor $gpg_pass_flags --detach-sign --output %{__signature_filename} %{__plaintext_filename}
EOF

map_rpm() {
  local goarch="$1"
  local rpmarch="$2"
  local rpm_file="$repo/rpm/$rpmarch/bast-${version}-1.${rpmarch}.rpm"
  mkdir -p "$repo/rpm/$rpmarch"
  cp "$dist_dir/bast_${version}_linux_${goarch}.rpm" "$rpm_file"
  if command -v rpmsign >/dev/null 2>&1; then
    HOME="$rpm_home" rpmsign --addsign "$rpm_file"
  elif command -v rpm >/dev/null 2>&1; then
    HOME="$rpm_home" rpm --addsign "$rpm_file"
  else
    echo "rpm/rpmsign is required to sign rpm packages" >&2
    return 1
  fi
  "$createrepo_bin" "$repo/rpm/$rpmarch"
  sign_file "$repo/rpm/$rpmarch/repodata/repomd.xml"
}

map_rpm amd64 x86_64
map_rpm arm64 aarch64
cp "$repo_root/.github/packaging/bast.repo" "$repo/bast.repo"

# --- arch ---
publish_arch() {
  local goarch="$1"
  local arch="$2"
  local src="$dist_dir/bast_${version}_linux_${goarch}.pkg.tar.zst"
  if [[ ! -f "$src" ]]; then
    echo "skipping arch $arch repo: missing $src"
    return 0
  fi
  mkdir -p "$repo/arch/$arch"
  local dest="$repo/arch/$arch/bast-${version}-1-${arch}.pkg.tar.zst"
  cp "$src" "$dest"
  gpg --batch --yes "${passphrase_args[@]}" --detach-sign --no-armor "$dest"

  if command -v repo-add >/dev/null 2>&1; then
    (cd "$repo/arch/$arch" && repo-add bast.db.tar.gz "$(basename "$dest")")
  elif command -v docker >/dev/null 2>&1; then
    docker run --rm \
      -v "$repo/arch/$arch:/work" \
      -w /work \
      archlinux:latest \
      bash -c "repo-add bast.db.tar.gz $(basename "$dest")"
  else
    echo "repo-add or docker is required to publish the Arch repository" >&2
    return 1
  fi
  local db
  for db in "$repo/arch/$arch"/bast.db.tar.gz "$repo/arch/$arch"/bast.files.tar.gz; do
    if [[ -f "$db" ]]; then
      gpg --batch --yes "${passphrase_args[@]}" --detach-sign --no-armor "$db"
    fi
  done
}

if [[ -f "$dist_dir/bast_${version}_linux_amd64.pkg.tar.zst" ]]; then
  publish_arch amd64 x86_64
  publish_arch arm64 aarch64
fi

# --- apk ---
publish_apk() {
  if [[ -z "${LINUX_APK_RSA_PRIVATE:-}" || -z "${LINUX_APK_RSA_PUBLIC:-}" ]]; then
    echo "LINUX_APK_RSA_PRIVATE/PUBLIC not set; skipping apk repository (packages still attach to the GitHub release)"
    return 0
  fi
  if [[ ! -f "$dist_dir/bast_${version}_linux_amd64.apk" ]]; then
    echo "skipping apk repo: missing apk packages"
    return 0
  fi
  if ! command -v docker >/dev/null 2>&1; then
    echo "docker is required to publish the apk repository; skipping"
    return 0
  fi

  decode_key "$LINUX_APK_RSA_PRIVATE" "$work/apk.rsa"
  decode_key "$LINUX_APK_RSA_PUBLIC" "$repo/bast.rsa.pub"
  chmod 600 "$work/apk.rsa"

  publish_apk_arch() {
    local goarch="$1"
    local apkarch="$2"
    mkdir -p "$repo/apk/$apkarch"
    cp "$dist_dir/bast_${version}_linux_${goarch}.apk" "$repo/apk/$apkarch/bast-${version}-r1.apk"
    docker run --rm \
      -v "$repo/apk/$apkarch:/work" \
      -v "$work/apk.rsa:/keys/bast.rsa:ro" \
      -v "$repo/bast.rsa.pub:/keys/bast.rsa.pub:ro" \
      -w /work \
      alpine:3.21 \
      sh -c "apk add --no-cache alpine-sdk >/dev/null && abuild-sign -k /keys/bast.rsa bast-${version}-r1.apk && apk index -o APKINDEX.tar.gz *.apk && abuild-sign -k /keys/bast.rsa APKINDEX.tar.gz"
  }

  publish_apk_arch amd64 x86_64
  publish_apk_arch arm64 aarch64
}

publish_apk

install -m 0755 "$repo_root/.github/packaging/setup.sh" "$repo/setup.sh"

if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI is required to upload the repository to R2" >&2
  exit 1
fi

export AWS_ACCESS_KEY_ID="$PACKAGES_R2_ACCESS_KEY_ID"
export AWS_SECRET_ACCESS_KEY="$PACKAGES_R2_SECRET_ACCESS_KEY"
export AWS_DEFAULT_REGION="auto"
endpoint="https://${PACKAGES_R2_ACCOUNT_ID}.r2.cloudflarestorage.com"

aws s3 sync "$repo" "s3://${PACKAGES_R2_BUCKET}" \
  --endpoint-url "$endpoint" \
  --only-show-errors

echo "Published Linux packages for $tag to ${BAST_PACKAGES_URL}"
