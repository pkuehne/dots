#!/bin/bash
# Scenario: tool install via GitHub releases.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 08: tools (GitHub release install) ==="

mkdir -p ~/gh-repo/files
cat > ~/gh-repo/dots.toml <<'TOML'
[meta]
version = 1

[tools]
bin_dir = "~/.local/bin"

[[tool]]
name = "fd"
desc = "Fast find replacement"
check = "fd --version"
install = [
    { method = "github", repo = "sharkdp/fd", asset = "fd-v{version}-{arch}-unknown-linux-musl.tar.gz", binary = "fd" },
]
TOML

mkdir -p ~/.local/bin

# ── check before install ──
output=$(dots --repo ~/gh-repo tools check 2>&1)
assert_contains "fd not installed" "✗" "$output"

# ── install from github ──
dots --repo ~/gh-repo tools install
assert_file_exists "fd binary exists" ~/.local/bin/fd
assert_file_mode "fd is executable" "755" ~/.local/bin/fd
assert_command_succeeds "fd runs" ~/.local/bin/fd --version

# ── check after install ──
output=$(dots --repo ~/gh-repo tools check 2>&1)
assert_contains "fd installed" "✓" "$output"

summary
