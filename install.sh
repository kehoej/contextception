#!/bin/sh
# Contextception installer
# Usage: curl -fsSL https://raw.githubusercontent.com/kehoej/contextception/main/install.sh | sh
set -e

REPO="kehoej/contextception"
INSTALL_DIR="/usr/local/bin"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)
    echo "Error: Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  linux|darwin) ;;
  *)
    echo "Error: Unsupported OS: $OS"
    echo "For Windows, download the binary from GitHub Releases."
    exit 1
    ;;
esac

# Get latest version
echo "Fetching latest version..."
VERSION=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)

if [ -z "$VERSION" ]; then
  echo "Error: Could not determine latest version"
  exit 1
fi

FILENAME="contextception_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

echo "Installing contextception ${VERSION} for ${OS}/${ARCH}..."

# Download and extract
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

curl -sL "$URL" -o "${TMPDIR}/${FILENAME}"

if [ ! -s "${TMPDIR}/${FILENAME}" ]; then
  echo "Error: Download failed"
  exit 1
fi

tar xzf "${TMPDIR}/${FILENAME}" -C "$TMPDIR"

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMPDIR}/contextception" "${INSTALL_DIR}/contextception"
else
  echo "Installing to ${INSTALL_DIR} requires elevated privileges."
  sudo mv "${TMPDIR}/contextception" "${INSTALL_DIR}/contextception"
fi

chmod +x "${INSTALL_DIR}/contextception"

echo ""
echo "contextception ${VERSION} installed to ${INSTALL_DIR}/contextception"
echo ""
echo "Get started:"
echo "  cd your-repo"
echo "  contextception index"
echo "  contextception analyze src/main.py"
echo ""
echo "Set up MCP (Claude Code):"
echo '  Add to ~/.claude/settings.json:'
echo '  { "mcpServers": { "contextception": { "command": "contextception", "args": ["mcp"] } } }'
