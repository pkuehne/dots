#!/bin/bash
# Scenario: init a repo, add files, apply, verify symlinks and idempotency.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 01: init and apply ==="

# ── init ──
dots init ~/dotfiles
assert_file_exists "dots init creates dots.toml" ~/dotfiles/dots.toml
assert_file_exists "dots init creates files/" ~/dotfiles/files

# ── add files ──
mkdir -p ~/dotfiles/files/.config/nvim
echo "set nocompatible" > ~/dotfiles/files/.vimrc
echo "-- nvim config" > ~/dotfiles/files/.config/nvim/init.lua
echo "[user]" > ~/dotfiles/files/.gitconfig

# ── dry run ──
output=$(dots --repo ~/dotfiles apply --dry-run 2>&1)
assert_contains "dry run mentions vimrc" ".vimrc" "$output"
assert_not_exists "dry run creates no files" ~/.vimrc

# ── apply ──
dots --repo ~/dotfiles apply
assert_symlink ".vimrc is a symlink" ~/.vimrc
assert_symlink "nested config is a symlink" ~/.config/nvim/init.lua
assert_eq ".vimrc content correct" "set nocompatible" "$(cat ~/.vimrc)"

# ── idempotency ──
inode_before=$(stat -c '%i' ~/.vimrc 2>/dev/null || stat -f '%i' ~/.vimrc)
dots --repo ~/dotfiles apply
inode_after=$(stat -c '%i' ~/.vimrc 2>/dev/null || stat -f '%i' ~/.vimrc)
assert_eq "apply is idempotent (same inode)" "$inode_before" "$inode_after"

# ── status ──
output=$(dots --repo ~/dotfiles status 2>&1)
assert_contains "status shows LINK" "LINK" "$output"

# ── backup on conflict ──
rm ~/.gitconfig  # remove symlink
echo "old content" > ~/.gitconfig  # create regular file
dots --repo ~/dotfiles apply
assert_file_exists "backup created" ~/.gitconfig.dots-bak
assert_eq "backup has old content" "old content" "$(cat ~/.gitconfig.dots-bak)"
assert_symlink "replaced with symlink" ~/.gitconfig

summary
