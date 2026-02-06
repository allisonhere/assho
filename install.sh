#!/bin/bash
set -e

REPO="allisonhere/asshi"
BINARY="asshi"
INSTALL_DIR="/usr/local/bin"

# Detect OS and Arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

if [[ "$ARCH" == "x86_64" ]]; then
    ARCH="amd64"
elif [[ "$ARCH" == "aarch64" ]] || [[ "$ARCH" == "arm64" ]]; then
    ARCH="arm64"
else
    echo "Unsupported architecture: $ARCH"
    exit 1
fi

# Construct asset name (matches what we built in release.yml)
# e.g., asshi-linux-amd64
ASSET_NAME="${BINARY}-${OS}-${ARCH}"

echo "Detected: $OS $ARCH"
echo "Fetching latest version from GitHub..."

# Get latest release tag
LATEST_TAG=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [[ -z "$LATEST_TAG" ]]; then
    echo "Error: Could not find latest release."
    exit 1
fi

DOWNLOAD_URL="https://github.com/$REPO/releases/download/$LATEST_TAG/$ASSET_NAME"

echo "Downloading $ASSET_NAME ($LATEST_TAG)..."
curl -sL "$DOWNLOAD_URL" -o "$BINARY"

chmod +x "$BINARY"

echo "Installing to $INSTALL_DIR (requires sudo)..."
if sudo mv "$BINARY" "$INSTALL_DIR/$BINARY"; then
    echo "Success! Installed to $INSTALL_DIR/$BINARY"
else
    echo "Failed to move binary. Do you have sudo permissions?"
    exit 1
fi

echo "Running asshi..."
exec "$BINARY"
