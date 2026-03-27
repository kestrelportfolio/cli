#!/usr/bin/env bash
# Kestrel CLI installer
#
# Usage:
#   curl -fsSL https://kestrelportfolio.com/install-cli | bash
#
# What it does:
#   1. Detects your OS and architecture
#   2. Downloads the latest release from GitHub
#   3. Installs the binary to /usr/local/bin (or ~/.local/bin if no sudo)
#   4. Verifies it works

set -euo pipefail

REPO="kestrelportfolio/cli"
BINARY_NAME="kestrel"

# --- Helper functions ---

info() { echo "  $*"; }
error() { echo "Error: $*" >&2; exit 1; }

detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux)  echo "linux" ;;
    *)      error "Unsupported OS: $(uname -s). Kestrel supports macOS and Linux." ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)  echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)             error "Unsupported architecture: $(uname -m). Kestrel supports amd64 and arm64." ;;
  esac
}

latest_version() {
  # Uses the GitHub API to get the latest release tag
  local url="https://api.github.com/repos/${REPO}/releases/latest"
  if command -v curl &>/dev/null; then
    curl -fsSL "$url" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//'
  elif command -v wget &>/dev/null; then
    wget -qO- "$url" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//'
  else
    error "Neither curl nor wget found. Install one and try again."
  fi
}

install_dir() {
  # Prefer /usr/local/bin if writable, otherwise ~/.local/bin
  if [ -w /usr/local/bin ]; then
    echo "/usr/local/bin"
  elif [ -w "$HOME/.local/bin" ]; then
    echo "$HOME/.local/bin"
  else
    # Try with sudo
    echo "/usr/local/bin"
  fi
}

# --- Main ---

echo "Installing Kestrel CLI..."
echo

OS=$(detect_os)
ARCH=$(detect_arch)
info "Detected: ${OS}/${ARCH}"

VERSION=$(latest_version)
if [ -z "$VERSION" ]; then
  error "Could not determine latest version. Check https://github.com/${REPO}/releases"
fi
info "Latest version: ${VERSION}"

# Strip the leading 'v' for the archive name
VERSION_NUM="${VERSION#v}"
ARCHIVE="${BINARY_NAME}_${VERSION_NUM}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

info "Downloading ${URL}..."

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

if command -v curl &>/dev/null; then
  curl -fsSL "$URL" -o "${TMPDIR}/${ARCHIVE}"
elif command -v wget &>/dev/null; then
  wget -q "$URL" -O "${TMPDIR}/${ARCHIVE}"
fi

info "Extracting..."
tar -xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

DEST=$(install_dir)
if [ -w "$DEST" ]; then
  mv "${TMPDIR}/${BINARY_NAME}" "${DEST}/${BINARY_NAME}"
else
  info "Need sudo to install to ${DEST}"
  sudo mv "${TMPDIR}/${BINARY_NAME}" "${DEST}/${BINARY_NAME}"
fi
chmod +x "${DEST}/${BINARY_NAME}"

info "Installed to ${DEST}/${BINARY_NAME}"
echo

# Verify
if command -v kestrel &>/dev/null; then
  echo "✓ $(kestrel version)"
  echo
  echo "Next steps:"
  echo "  kestrel login          Authenticate with your API token"
  echo "  kestrel setup claude   Set up Claude Code integration"
else
  echo "✓ Installed, but ${DEST} may not be in your PATH."
  echo "  Add it with: export PATH=\"${DEST}:\$PATH\""
fi
