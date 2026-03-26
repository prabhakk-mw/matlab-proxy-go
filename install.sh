#!/bin/sh
# Copyright 2026 The MathWorks, Inc.
#
# Install script for matlab-proxy.
# Downloads the latest release binary from GitHub and installs it to ~/.local/bin.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/prabhakk-mw/matlab-proxy-go/main/install.sh | sh
#
# Options (via environment variables):
#   VERSION     Install a specific version (e.g. VERSION=0.5.1)
#   INSTALL_DIR Install to a custom directory (default: $HOME/.local/bin)
#
# Note: When piping to sh, pass env vars to sh (right side of pipe), not curl:
#   curl -fsSL ... | VERSION=0.5.1 INSTALL_DIR=. sh

set -e

REPO="prabhakk-mw/matlab-proxy-go"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS
OS="$(uname -s)"
case "$OS" in
    Linux)  OS="linux" ;;
    Darwin) OS="darwin" ;;
    *)      echo "Error: Unsupported operating system: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64)  ARCH="arm64" ;;
    *)              echo "Error: Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Determine version
if [ -z "${VERSION:-}" ]; then
    echo "Fetching latest release..."
    VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')"
    if [ -z "$VERSION" ]; then
        echo "Error: Could not determine latest version. Set VERSION manually."
        exit 1
    fi
fi

TAG="v${VERSION}"
ARCHIVE="matlab-proxy-${TAG}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"

echo "Installing matlab-proxy ${VERSION} (${OS}/${ARCH})..."
echo "  From: ${URL}"
echo "  To:   ${INSTALL_DIR}/matlab-proxy"

# Download and extract
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

curl -fSL -o "${TMPDIR}/${ARCHIVE}" "$URL"
tar xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

# Install
mkdir -p "$INSTALL_DIR"
if [ -w "$INSTALL_DIR" ]; then
    mv "${TMPDIR}/matlab-proxy" "${INSTALL_DIR}/matlab-proxy"
else
    echo "  (requires sudo to write to ${INSTALL_DIR})"
    sudo mv "${TMPDIR}/matlab-proxy" "${INSTALL_DIR}/matlab-proxy"
fi

chmod +x "${INSTALL_DIR}/matlab-proxy"

echo ""
echo "matlab-proxy ${VERSION} installed successfully."
"${INSTALL_DIR}/matlab-proxy" --version
