#!/bin/bash
# Scenario: shell managed mode — env, path, snippets.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 04: shell managed mode ==="

mkdir -p ~/shell-repo/files ~/shell-repo/shell
cat > ~/shell-repo/dots.toml <<'TOML'
[meta]
version = 1

[env]
EDITOR = "nvim"
LANG = "en_US.UTF-8"

[shell]
managed = true
dir = "~/.config/dots/shell.d"
path = ["~/.cargo/bin", "~/.local/bin"]
TOML

echo '# my aliases' > ~/shell-repo/shell/30-aliases.sh

# ── apply ──
dots --repo ~/shell-repo apply

# env snippet
assert_file_exists "010-env.sh generated" ~/.config/dots/shell.d/010-env.sh
env_content=$(cat ~/.config/dots/shell.d/010-env.sh)
assert_contains "env has EDITOR" 'export EDITOR="nvim"' "$env_content"
assert_contains "env has LANG" 'export LANG="en_US.UTF-8"' "$env_content"
assert_contains "env is sorted" "EDITOR" "$env_content"

# path snippet
assert_file_exists "020-path.sh generated" ~/.config/dots/shell.d/020-path.sh
path_content=$(cat ~/.config/dots/shell.d/020-path.sh)
assert_contains "path has cargo" ".cargo/bin" "$path_content"
assert_contains "path has local bin" ".local/bin" "$path_content"
assert_contains "path uses case guard" 'case ":$PATH:"' "$path_content"

# user snippet deployed
assert_file_exists "user snippet deployed" ~/.config/dots/shell.d/30-aliases.sh

# ── bootstrapper installed by apply ──
assert_file_exists ".zshrc exists" ~/.zshrc
zshrc=$(cat ~/.zshrc)
assert_contains "bootstrapper in .zshrc" "dots managed" "$zshrc"
assert_file_exists ".bashrc exists" ~/.bashrc
bashrc=$(cat ~/.bashrc)
assert_contains "bootstrapper in .bashrc" "dots managed" "$bashrc"

# ── shell show ──
output=$(dots --repo ~/shell-repo shell show 2>&1)
assert_contains "shell show has env" "EDITOR" "$output"

summary
