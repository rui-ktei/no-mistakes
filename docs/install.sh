#!/bin/sh
set -e

REPO="kunchenguid/no-mistakes"
INSTALL_DIR="${NO_MISTAKES_INSTALL_DIR:-$HOME/.no-mistakes/bin}"
LINK_DIR="${NO_MISTAKES_LINK_DIR:-}"

if [ -z "$LINK_DIR" ]; then
  case ":$PATH:" in
    *":$HOME/.local/bin:"*) LINK_DIR="$HOME/.local/bin" ;;
    *) LINK_DIR="/usr/local/bin" ;;
  esac
fi

BIN_PATH="$INSTALL_DIR/no-mistakes"
LINK_PATH="$LINK_DIR/no-mistakes"
NOM_LINK_PATH="$LINK_DIR/nom"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
if [ -z "$VERSION" ]; then
  echo "Could not determine latest release"
  exit 1
fi

FILENAME="no-mistakes-${VERSION}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading no-mistakes ${VERSION} for ${OS}/${ARCH}..."
curl -fsSL "$URL" -o "${TMPDIR}/${FILENAME}"
tar xzf "${TMPDIR}/${FILENAME}" -C "$TMPDIR"

if ! mkdir -p "$INSTALL_DIR"; then
  echo "Could not create install directory: $INSTALL_DIR"
  exit 1
fi

mv "${TMPDIR}/no-mistakes" "$BIN_PATH"
chmod 755 "$BIN_PATH" 2>/dev/null || true

resolve_path() {
  (cd "$1" 2>/dev/null && pwd -P)
}

# Create a symlink, falling back to sudo when the target directory is not
# writable. $1 = link path, $2 = link target.
make_symlink() {
  symlink_dir="$(dirname "$1")"
  if [ -w "$symlink_dir" ] || (mkdir -p "$symlink_dir" 2>/dev/null && [ -w "$symlink_dir" ]); then
    rm -f "$1"
    ln -s "$2" "$1"
  else
    echo "Linking $1 -> $2 (requires sudo)..."
    sudo mkdir -p "$symlink_dir"
    sudo rm -f "$1"
    sudo ln -s "$2" "$1"
  fi
}

REAL_INSTALL_DIR="$(resolve_path "$INSTALL_DIR")"
REAL_LINK_DIR="$(resolve_path "$LINK_DIR" 2>/dev/null || echo "")"

if [ -n "$REAL_INSTALL_DIR" ] && [ "$REAL_INSTALL_DIR" = "$REAL_LINK_DIR" ]; then
  echo "Install dir and link dir resolve to the same path; skipping no-mistakes symlink."
  # The binary is already on PATH as no-mistakes; still provide the short nom alias.
  NOM_LINK_PATH="$INSTALL_DIR/nom"
  make_symlink "$NOM_LINK_PATH" "no-mistakes"
else
  make_symlink "$LINK_PATH" "$BIN_PATH"
  make_symlink "$NOM_LINK_PATH" "$BIN_PATH"
fi

echo "no-mistakes ${VERSION} installed to ${BIN_PATH}"
echo "Command path: ${LINK_PATH} -> ${BIN_PATH}"
echo "Short alias:  ${NOM_LINK_PATH} (nom)"

"$BIN_PATH" daemon restart >/dev/null

case ":$PATH:" in
  *":$LINK_DIR:"*) ;;
  *) echo "Add ${LINK_DIR} to your PATH and restart your terminal." ;;
esac
