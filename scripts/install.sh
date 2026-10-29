#!/usr/bin/env bash
# Install a prebuilt pmock-cli from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/haidaisiganbei/pmock-cli/main/scripts/install.sh | sh
#
# Overrides:
#   PMOCK_CLI_VERSION       e.g. 0.1.2 (default: latest release)
#   PMOCK_CLI_INSTALL_DIR   default: $HOME/.local/bin
set -euo pipefail

REPO="${PMOCK_CLI_REPO:-haidaisiganbei/pmock-cli}"
VERSION="${PMOCK_CLI_VERSION:-}"
INSTALL_DIR="${PMOCK_CLI_INSTALL_DIR:-$HOME/.local/bin}"

case "$(uname -s)" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *) echo "unsupported OS: $(uname -s); use the Windows zip from Releases" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  arm64|aarch64) arch="arm64" ;;
  x86_64|amd64) arch="amd64" ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

if [ -z "$VERSION" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -1)"
fi
if [ -z "$VERSION" ]; then
  echo "failed to resolve the latest release; set PMOCK_CLI_VERSION" >&2
  exit 1
fi

base="https://github.com/$REPO/releases/download/v$VERSION"
asset="pmock-cli_${VERSION}_${os}_${arch}.tar.gz"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "downloading $asset"
curl -fsSL "$base/$asset" -o "$tmp/$asset"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

expected="$(grep "  $asset\$" "$tmp/checksums.txt" | awk '{print $1}')"
if [ -z "$expected" ]; then
  echo "checksum missing for $asset" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/$asset" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')"
fi
if [ "$expected" != "$actual" ]; then
  echo "checksum mismatch for $asset" >&2
  exit 1
fi

tar -xzf "$tmp/$asset" -C "$tmp"
mkdir -p "$INSTALL_DIR"
install -m755 "$tmp/pmock-cli" "$INSTALL_DIR/pmock-cli"

echo "installed $INSTALL_DIR/pmock-cli ($VERSION)"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "add to PATH: export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac
"$INSTALL_DIR/pmock-cli" --version
