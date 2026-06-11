#!/bin/sh
# Bootstrap installer for dots.
# Usage: curl -fsSL https://raw.githubusercontent.com/pkuehne/dots/refs/heads/main/install.sh | sh
set -eu

REPO="pkuehne/dots"
BIN_DIR="${DOTS_BIN_DIR:-$HOME/.local/bin}"

info()  { printf '  \033[1;34m→\033[0m %s\n' "$1"; }
ok()    { printf '  \033[1;32m✓\033[0m %s\n' "$1"; }
fail()  { printf '  \033[1;31m✗\033[0m %s\n' "$1" >&2; exit 1; }

# ── preflight ──

command -v curl >/dev/null 2>&1 || fail "curl not found — install curl first"

# ── detect platform/arch ──

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)       fail "Unsupported architecture: $ARCH" ;;
esac
case "$OS" in
    linux|darwin) ;;
    *)            fail "Unsupported OS: $OS" ;;
esac

info "Detected: $OS/$ARCH"

# ── fetch latest release ──

RELEASE_URL="https://api.github.com/repos/$REPO/releases/latest"
ASSET_NAME="dots_${OS}_${ARCH}"
DOWNLOAD_URL=$(curl -fsSL "$RELEASE_URL" | grep "browser_download_url" | grep "$ASSET_NAME" | cut -d '"' -f4)

if [ -z "$DOWNLOAD_URL" ]; then
    fail "Could not find release asset for $OS/$ARCH. Check https://github.com/$REPO/releases"
fi

info "Downloading $ASSET_NAME"
TMP=$(mktemp)
curl -fsSL -o "$TMP" "$DOWNLOAD_URL"

# ── install ──

mkdir -p "$BIN_DIR"
install -m 755 "$TMP" "$BIN_DIR/dots"
rm "$TMP"
ok "Installed dots → $BIN_DIR/dots"

# ── check PATH ──

case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *)
        printf '\n'
        info "Add this to your shell profile:"
        printf '    export PATH="%s:$PATH"\n' "$BIN_DIR"
        printf '\n'
        ;;
esac

printf '\n'
ok "Done! Run 'dots --version' to verify."
