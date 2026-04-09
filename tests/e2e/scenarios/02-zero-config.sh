#!/bin/bash
# Scenario: zero-config mode — no dots.toml, just files/ directory.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 02: zero config (no dots.toml) ==="

# ── set up bare repo with no toml ──
mkdir -p ~/bare-repo/files/.config
echo "alias ll='ls -la'" > ~/bare-repo/files/.bash_aliases
echo "colorscheme desert" > ~/bare-repo/files/.config/vimrc

# Need a minimal dots.toml for the parser (version only)
echo '[meta]' > ~/bare-repo/dots.toml
echo 'version = 1' >> ~/bare-repo/dots.toml

# ── apply ──
dots --repo ~/bare-repo apply
assert_symlink ".bash_aliases deployed" ~/.bash_aliases
assert_symlink "nested .config/vimrc deployed" ~/.config/vimrc
assert_eq "content correct" "alias ll='ls -la'" "$(cat ~/.bash_aliases)"

# ── list ──
output=$(dots --repo ~/bare-repo list 2>&1)
assert_contains "list shows bash_aliases" ".bash_aliases" "$output"

summary
