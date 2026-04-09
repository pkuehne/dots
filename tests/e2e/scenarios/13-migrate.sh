#!/bin/bash
# Scenario: migrate discovers existing dotfiles.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 13: migrate ==="

# ── create some dotfiles in ~ ──
echo "# my zshrc" > ~/.zshrc
echo "# my bashrc" > ~/.bashrc
mkdir -p ~/.config/git
echo "[user]" > ~/.config/git/config

# ── init a repo ──
dots init ~/mig-repo

# ── migrate dry run ──
output=$(dots --repo ~/mig-repo migrate 2>&1)
assert_contains "migrate finds .zshrc" ".zshrc" "$output"
assert_contains "migrate finds .bashrc" ".bashrc" "$output"

# ── migrate --write ──
dots --repo ~/mig-repo migrate --write
assert_file_exists ".zshrc copied to repo" ~/mig-repo/files/.zshrc
assert_eq ".zshrc content preserved" "# my zshrc" "$(cat ~/mig-repo/files/.zshrc)"

summary
