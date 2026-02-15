#!/usr/bin/env bash
# file: install.sh
# version: 1.0.0
# guid: 3b2a1c0d-9e8f-4765-b4c3-2d1e0f9a8b7c

set -euo pipefail

REPO="jdfalk/audiobook-organizer"
BINARY_NAME="audiobook-organizer"
INSTALL_DIR="/usr/local/bin"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command '$1' not found" >&2
    exit 1
  fi
}

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
      echo "error: unsupported architecture '$arch'" >&2
      exit 1
      ;;
  esac

  case "$os" in
    linux|darwin) ;;
    *)
      echo "error: unsupported OS '$os'" >&2
      exit 1
      ;;
  esac

  printf '%s_%s' "$os" "$arch"
}

latest_tag() {
  require_cmd curl
  require_cmd grep
  local tag
  tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep -m1 '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')"
  if [[ -z "$tag" ]]; then
    echo "error: failed to determine latest release tag" >&2
    exit 1
  fi
  printf '%s' "$tag"
}

download_and_install() {
  require_cmd curl
  require_cmd tar

  local platform tag archive url tmpdir
  platform="$(detect_platform)"
  tag="$(latest_tag)"
  archive="${BINARY_NAME}_${platform}.tar.gz"
  url="https://github.com/${REPO}/releases/download/${tag}/${archive}"
  tmpdir="$(mktemp -d)"

  cleanup() {
    rm -rf "$tmpdir"
  }
  trap cleanup EXIT

  echo "Downloading ${url}"
  curl -fsSL "$url" -o "${tmpdir}/${archive}"
  tar -xzf "${tmpdir}/${archive}" -C "$tmpdir"

  if [[ ! -f "${tmpdir}/${BINARY_NAME}" ]]; then
    echo "error: archive did not contain ${BINARY_NAME}" >&2
    exit 1
  fi

  if [[ ! -w "$INSTALL_DIR" ]]; then
    echo "Install dir not writable; requesting sudo to place binary in ${INSTALL_DIR}"
    sudo install -m 0755 "${tmpdir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  else
    install -m 0755 "${tmpdir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  fi

  echo "Installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
  echo "Quick start: ${BINARY_NAME} serve --dir /path/to/audiobooks --host 0.0.0.0"
}

download_and_install
