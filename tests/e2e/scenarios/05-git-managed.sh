#!/bin/bash
# Scenario: git managed mode.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 05: git managed mode ==="

mkdir -p ~/git-repo/files
cat > ~/git-repo/dots.toml <<'TOML'
[meta]
version = 1

[git]
managed = true
name = "Test User"
email = "test@dots.dev"
editor = "nvim"
default_branch = "main"
TOML

# ── git init ──
dots --repo ~/git-repo git init
assert_file_exists "managed.gitconfig created" ~/.config/dots/git/managed.gitconfig
gc=$(cat ~/.config/dots/git/managed.gitconfig)
assert_contains "has user name" "Test User" "$gc"
assert_contains "has user email" "test@dots.dev" "$gc"
assert_contains "has editor" "nvim" "$gc"

# Check include was added to ~/.gitconfig
assert_file_exists ".gitconfig exists" ~/.gitconfig
gitconfig=$(cat ~/.gitconfig)
assert_contains "include added" "managed.gitconfig" "$gitconfig"

# ── git show ──
output=$(dots --repo ~/git-repo git show 2>&1)
assert_contains "git show has user" "Test User" "$output"

# ── git uninit ──
dots --repo ~/git-repo git uninit
gitconfig_after=$(cat ~/.gitconfig)
assert_eq "include removed" "false" "$(echo "$gitconfig_after" | grep -q 'managed.gitconfig' && echo true || echo false)"

summary
