#!/usr/bin/env sh
# install.sh — download the latest dsc release binary into ~/.local/bin
# (or $PREFIX/bin when PREFIX is set). Idempotent.
#
# Usage:
#   curl -fsSL https://deepseekcode.dev/install.sh | sh
#   curl -fsSL https://deepseekcode.dev/install.sh | DSC_VERSION=v0.1.0 sh

set -eu

REPO="amemiya02/deepseekcode"
BIN_NAME="dsc"
PREFIX="${PREFIX:-$HOME/.local}"
DEST_DIR="$PREFIX/bin"
VERSION="${DSC_VERSION:-}"

err() { printf >&2 "install.sh: %s\n" "$*"; exit 1; }
note() { printf "==> %s\n" "$*"; }

detect_platform() {
  uname_os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$uname_os" in
    linux)  os=linux ;;
    darwin) os=darwin ;;
    msys*|mingw*|cygwin*) os=windows ;;
    *) err "unsupported OS: $uname_os" ;;
  esac

  uname_arch="$(uname -m)"
  case "$uname_arch" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) err "unsupported arch: $uname_arch" ;;
  esac

  printf "%s %s\n" "$os" "$arch"
}

resolve_version() {
  if [ -n "$VERSION" ]; then
    printf "%s\n" "$VERSION"
    return
  fi
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
      | grep '"tag_name":' \
      | head -1 \
      | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/'
    return
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO- "https://api.github.com/repos/$REPO/releases/latest" \
      | grep '"tag_name":' \
      | head -1 \
      | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/'
    return
  fi
  err "need curl or wget to resolve latest version"
}

fetch() {
  url="$1"
  out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$out" "$url" || return 1
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url" || return 1
  else
    err "need curl or wget"
  fi
}

main() {
  set -- $(detect_platform)
  os="$1"; arch="$2"

  version="$(resolve_version)"
  [ -z "$version" ] && err "could not resolve version"
  version_no_v="${version#v}"
  note "installing deepseekcode $version for $os/$arch"

  if [ "$os" = "windows" ]; then
    ext="zip"
  else
    ext="tar.gz"
  fi
  asset="deepseekcode_${version_no_v}_${os}_${arch}.${ext}"
  url="https://github.com/$REPO/releases/download/$version/$asset"

  tmpdir="$(mktemp -d 2>/dev/null || mktemp -d -t dsc-install)"
  trap 'rm -rf "$tmpdir"' EXIT
  archive="$tmpdir/$asset"

  note "fetching $url"
  fetch "$url" "$archive" || err "download failed"

  case "$ext" in
    tar.gz) (cd "$tmpdir" && tar -xzf "$archive") ;;
    zip)    (cd "$tmpdir" && unzip -q "$archive") ;;
  esac

  binary="$tmpdir/$BIN_NAME"
  [ "$os" = "windows" ] && binary="$tmpdir/${BIN_NAME}.exe"
  [ -f "$binary" ] || err "binary not found in archive: $binary"

  mkdir -p "$DEST_DIR"
  cp "$binary" "$DEST_DIR/$BIN_NAME"
  chmod +x "$DEST_DIR/$BIN_NAME"
  note "installed: $DEST_DIR/$BIN_NAME"

  case ":$PATH:" in
    *":$DEST_DIR:"*) ;;
    *)
      note "NOTE: $DEST_DIR is not on your PATH."
      note "Add this to your shell profile:"
      printf '\n  export PATH="%s:$PATH"\n\n' "$DEST_DIR"
      ;;
  esac

  "$DEST_DIR/$BIN_NAME" -version
}

main "$@"
