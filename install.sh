#!/bin/sh
# sshu installer for macOS / Linux (unix-only — on Windows use WSL).
# Usage: curl -fsSL https://raw.githubusercontent.com/vulcanshen/sshu/main/install.sh | sh

set -e

REPO="vulcanshen/sshu"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux*)  OS="linux" ;;
  darwin*) OS="darwin" ;;
  *) echo "Error: sshu is unix-only (no Windows build — use WSL). Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Error: unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest version
echo "Fetching latest release..."
VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed 's/.*"v\(.*\)".*/\1/')
echo "Latest version: $VERSION"

# Install dir
if [ "$(id -u)" = "0" ]; then
  INSTALL_DIR="/usr/local/bin"
else
  INSTALL_DIR="$HOME/.local/bin"
fi

FILENAME="sshu_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/$REPO/releases/download/v${VERSION}/$FILENAME"

# Download
TMPDIR=$(mktemp -d)
echo "Downloading $FILENAME..."
curl -fsSL "$DOWNLOAD_URL" -o "$TMPDIR/$FILENAME"

# Extract
echo "Extracting..."
tar xzf "$TMPDIR/$FILENAME" -C "$TMPDIR"

# Install
mkdir -p "$INSTALL_DIR"
cp "$TMPDIR/sshu" "$INSTALL_DIR/sshu"
chmod +x "$INSTALL_DIR/sshu"
rm -rf "$TMPDIR"

echo ""
echo "sshu $VERSION installed to $INSTALL_DIR"

# Check if install dir is in PATH
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo ""
    echo "WARNING: $INSTALL_DIR is not in your PATH. Add it by running:"
    echo ""
    echo "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.$(basename "$SHELL")rc && source ~/.$(basename "$SHELL")rc"
    ;;
esac

# The ssh tab shells out to the system's OpenSSH client; every macOS/Linux box
# has one, but say so if this one somehow does not.
if ! command -v ssh >/dev/null 2>&1; then
  echo ""
  echo "NOTE: no 'ssh' client on PATH — the ssh tab needs OpenSSH (the sftp side does not)."
fi

echo ""
echo "NOTE: sshu draws auth methods, file types and marks with Nerd Font glyphs."
echo "      Use a Nerd Font terminal profile (https://www.nerdfonts.com) or those"
echo "      cells will render as boxes."
echo ""
echo "Run 'sshu' to launch."
