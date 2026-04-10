#!/bin/sh
# Bootstrap installer for dots.
# Usage: curl -fsSL https://raw.githubusercontent.com/pkuehne/dots/refs/heads/main/install.sh | sh
set -eu

REPO="https://github.com/pkuehne/dots.git"
INSTALL_DIR="${DOTS_INSTALL_DIR:-$HOME/.local/share/dots}"
BIN_DIR="${DOTS_BIN_DIR:-$HOME/.local/bin}"

info()  { printf '  \033[1;34m→\033[0m %s\n' "$1"; }
ok()    { printf '  \033[1;32m✓\033[0m %s\n' "$1"; }
fail()  { printf '  \033[1;31m✗\033[0m %s\n' "$1" >&2; exit 1; }

# ── preflight ──

command -v python3 >/dev/null 2>&1 || fail "python3 not found — install Python 3.10+ first"
command -v git     >/dev/null 2>&1 || fail "git not found — install git first"

py_version=$(python3 -c "import sys; print(f'{sys.version_info.major}.{sys.version_info.minor}')")
py_major=$(echo "$py_version" | cut -d. -f1)
py_minor=$(echo "$py_version" | cut -d. -f2)

if [ "$py_major" -lt 3 ] || { [ "$py_major" -eq 3 ] && [ "$py_minor" -lt 10 ]; }; then
    fail "Python $py_version found, but 3.10+ is required"
fi

ok "Python $py_version"

# ── clone or update ──

if [ -d "$INSTALL_DIR/.git" ]; then
    info "Updating existing install at $INSTALL_DIR"
    git -C "$INSTALL_DIR" pull --ff-only --quiet
    ok "Updated"
else
    info "Cloning dots into $INSTALL_DIR"
    mkdir -p "$(dirname "$INSTALL_DIR")"
    git clone --quiet "$REPO" "$INSTALL_DIR"
    ok "Cloned"
fi

# ── install into venv ──

VENV_DIR="$INSTALL_DIR/.venv"

if [ ! -d "$VENV_DIR" ]; then
    info "Creating virtual environment"
    python3 -m venv "$VENV_DIR"
    ok "Virtual environment created"
fi

info "Installing dots"
"$VENV_DIR/bin/pip" install --quiet "$INSTALL_DIR"
ok "Installed"

# ── create wrapper script ──

mkdir -p "$BIN_DIR"

cat > "$BIN_DIR/dots" <<WRAPPER
#!/bin/sh
exec "$VENV_DIR/bin/dots" "\$@"
WRAPPER
chmod +x "$BIN_DIR/dots"
ok "Linked dots -> $BIN_DIR/dots"

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
