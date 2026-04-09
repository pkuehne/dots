#!/bin/bash
# Scenario: tool check and install via apt.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 07: tools (apt install) ==="

mkdir -p ~/tools-repo/files
cat > ~/tools-repo/dots.toml <<'TOML'
[meta]
version = 1

[tools]
bin_dir = "~/.local/bin"

[[tool]]
name = "jq"
check = "jq --version"
tags = ["core"]
install = [
    { method = "apt", package = "jq", only = ["linux"] },
]

[[tool]]
name = "tree"
check = "tree --version"
tags = ["core"]
install = [
    { method = "apt", package = "tree", only = ["linux"] },
]
TOML

# ── tools list ──
output=$(dots --repo ~/tools-repo tools list 2>&1)
assert_contains "tools list shows jq" "jq" "$output"
assert_contains "tools list shows tree" "tree" "$output"

# ── tools check (before install) ──
output=$(dots --repo ~/tools-repo tools check 2>&1 || true)
assert_contains "jq not installed" "✗" "$output"

# ── tools install ──
dots --repo ~/tools-repo tools install 2>&1

# ── tools check (after install) ──
output=$(dots --repo ~/tools-repo tools check 2>&1 || true)
assert_contains "jq installed" "✓" "$output"

# ── verify binaries work ──
assert_command_succeeds "jq runs" jq --version
assert_command_succeeds "tree runs" tree --version

# ── install is idempotent ──
output=$(dots --repo ~/tools-repo tools install 2>&1 || true)
assert_contains "already installed" "✓" "$output"

summary
