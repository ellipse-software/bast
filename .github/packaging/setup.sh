#!/bin/sh
# Add the Bast package repository and install bast.
# Usage: curl -fsSL https://packages.bast.sh/setup.sh | sudo sh
#        curl -fsSL https://packages.bast.sh/setup.sh | sudo sh -s -- apt|dnf|zypper|pacman|apk

set -eu

PACKAGES_URL="${BAST_PACKAGES_URL:-https://packages.bast.sh}"
FAMILY="${1:-}"

fail() {
  printf 'setup.sh: %s\n' "$1" >&2
  exit 1
}

need_root() {
  if [ "$(id -u)" -ne 0 ]; then
    fail "run as root, for example: curl -fsSL ${PACKAGES_URL}/setup.sh | sudo sh"
  fi
}

download() {
  url="$1"
  dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$dest" "$url"
  else
    fail "curl or wget is required"
  fi
}

detect_family() {
  if [ -n "$FAMILY" ]; then
    printf '%s\n' "$FAMILY"
    return 0
  fi
  if [ ! -f /etc/os-release ]; then
    fail "cannot detect distro (missing /etc/os-release); pass apt, dnf, zypper, pacman, or apk"
  fi
  # shellcheck disable=SC1091
  . /etc/os-release
  ids="$(printf '%s %s' "${ID:-}" "${ID_LIKE:-}")"
  case "$ids" in
  *debian* | *ubuntu*)
    printf 'apt\n'
    ;;
  *fedora* | *rhel* | *centos* | *rocky* | *alma* | *amzn* | *ol*)
    printf 'dnf\n'
    ;;
  *suse* | *sles*)
    printf 'zypper\n'
    ;;
  *arch* | *manjaro* | *endeavouros*)
    printf 'pacman\n'
    ;;
  *alpine*)
    printf 'apk\n'
    ;;
  *)
    fail "unsupported distro ${ID:-unknown}; pass apt, dnf, zypper, pacman, or apk"
    ;;
  esac
}

install_apt() {
  mkdir -p /usr/share/keyrings
  download "$PACKAGES_URL/bast.gpg" /usr/share/keyrings/bast.gpg
  chmod 0644 /usr/share/keyrings/bast.gpg
  printf 'deb [signed-by=/usr/share/keyrings/bast.gpg] %s/apt stable main\n' "$PACKAGES_URL" \
    >/etc/apt/sources.list.d/bast.list
  apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y bast
}

install_dnf() {
  download "$PACKAGES_URL/bast.repo" /etc/yum.repos.d/bast.repo
  if command -v dnf >/dev/null 2>&1; then
    dnf install -y bast
  else
    yum install -y bast
  fi
}

install_zypper() {
  download "$PACKAGES_URL/bast.repo" /etc/zypp/repos.d/bast.repo
  zypper --gpg-auto-import-keys refresh bast
  zypper install -y bast
}

install_pacman() {
  tmp="$(mktemp)"
  download "$PACKAGES_URL/bast.asc" "$tmp"
  if command -v pacman-key >/dev/null 2>&1; then
    pacman-key --add "$tmp"
    fingerprint="$(gpg --show-keys --with-colons "$tmp" | awk -F: '/^fpr:/ { print $10; exit }')"
    if [ -n "$fingerprint" ]; then
      pacman-key --lsign-key "$fingerprint" >/dev/null 2>&1 || pacman-key --lsign-key "$fingerprint"
    fi
  fi
  rm -f "$tmp"
  if ! grep -q '^\[bast\]' /etc/pacman.conf; then
    printf '\n[bast]\nSigLevel = Required\nServer = %s/arch/$arch\n' "$PACKAGES_URL" >>/etc/pacman.conf
  fi
  pacman -Sy --noconfirm bast
}

install_apk() {
  mkdir -p /etc/apk/keys
  download "$PACKAGES_URL/bast.rsa.pub" /etc/apk/keys/bast.rsa.pub
  if ! grep -qxF "$PACKAGES_URL/apk" /etc/apk/repositories; then
    printf '%s/apk\n' "$PACKAGES_URL" >>/etc/apk/repositories
  fi
  apk update
  apk add bast
}

need_root
FAMILY="$(detect_family)"

case "$FAMILY" in
apt | deb | debian | ubuntu)
  install_apt
  ;;
dnf | yum | rpm | fedora | rhel)
  install_dnf
  ;;
zypper | suse)
  install_zypper
  ;;
pacman | arch)
  install_pacman
  ;;
apk | alpine)
  install_apk
  ;;
*)
  fail "unknown package family: $FAMILY"
  ;;
esac

printf 'Bast installed. Run bast to get started.\n'
