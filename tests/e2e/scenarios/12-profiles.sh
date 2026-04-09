#!/bin/bash
# Scenario: profile overrides.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 12: profiles ==="

mkdir -p ~/prof-repo/files
cat > ~/prof-repo/dots.toml <<'TOML'
[meta]
version = 1

[env]
EDITOR = "nvim"
LANG = "en_US.UTF-8"

[shell]
managed = true
path = ["~/.local/bin"]

[profiles.work]
env.EDITOR = "code"
env.HTTP_PROXY = "http://proxy:8080"
TOML

# ── apply without profile ──
dots --repo ~/prof-repo apply
env_content=$(cat ~/.config/dots/shell.d/010-env.sh)
assert_contains "default EDITOR is nvim" 'export EDITOR="nvim"' "$env_content"

# ── apply with work profile ──
dots --repo ~/prof-repo --profile work apply
env_content=$(cat ~/.config/dots/shell.d/010-env.sh)
assert_contains "work EDITOR is code" 'export EDITOR="code"' "$env_content"
assert_contains "work has proxy" 'export HTTP_PROXY="http://proxy:8080"' "$env_content"

summary
