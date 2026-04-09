#!/bin/bash
# Scenario: Termux platform detection and behaviour.
# This runs inside Dockerfile.termux where /data/data/com.termux exists.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 11: Termux platform ==="

# ── platform detection ──
plat=$(python3 -c "from dots.platform import detect_platform; print(detect_platform())")
assert_eq "detects termux platform" "termux" "$plat"

# ── init and apply ──
dots init ~/dotfiles
echo "termux config" > ~/dotfiles/files/.termux_config

dots --repo ~/dotfiles apply
assert_symlink "file deployed on termux" ~/.termux_config

# ── platform-specific files ──
mkdir -p ~/dotfiles/files.d/termux ~/dotfiles/files.d/linux
echo "termux only" > ~/dotfiles/files.d/termux/.termux-specific
echo "linux desktop" > ~/dotfiles/files.d/linux/.desktop-file

dots --repo ~/dotfiles apply
assert_symlink "termux-specific file deployed" ~/.termux-specific
assert_not_exists "linux file not deployed on termux" ~/.desktop-file

# ── env and shell ──
cat > ~/dotfiles/dots.toml <<'TOML'
[meta]
version = 1

[env]
EDITOR = "nano"

[shell]
managed = true
path = ["~/.local/bin"]
TOML

dots --repo ~/dotfiles apply
assert_file_exists "env snippet generated" ~/.config/dots/shell.d/010-env.sh
env_content=$(cat ~/.config/dots/shell.d/010-env.sh)
assert_contains "env has EDITOR" 'export EDITOR="nano"' "$env_content"

# ── doctor on termux ──
output=$(dots --repo ~/dotfiles doctor 2>&1 || true)
assert_contains "doctor runs on termux" "Python" "$output"

# ── tools: apt should be rejected on termux ──
cat > ~/dotfiles/dots.toml <<'TOML'
[meta]
version = 1

[[tool]]
name = "jq"
check = "jq --version"
install = [
    { method = "apt", package = "jq" },
]
TOML

# apt install should fail with a helpful error on termux
output=$(dots --repo ~/dotfiles tools install 2>&1 || true)
assert_contains "apt rejected on termux" "sudo" "$output"

summary
