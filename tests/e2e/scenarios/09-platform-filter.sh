#!/bin/bash
# Scenario: platform-specific files and only= guards.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 09: platform filtering ==="

mkdir -p ~/plat-repo/files ~/plat-repo/files.d/linux ~/plat-repo/files.d/darwin
echo "shared" > ~/plat-repo/files/.shared
echo "linux only" > ~/plat-repo/files.d/linux/.linux-file
echo "mac only" > ~/plat-repo/files.d/darwin/.darwin-file
echo -e '[meta]\nversion = 1' > ~/plat-repo/dots.toml

dots --repo ~/plat-repo apply

# We're in a Linux container
assert_symlink "shared file deployed" ~/.shared
assert_symlink "linux file deployed" ~/.linux-file
assert_not_exists "darwin file not deployed" ~/.darwin-file

# ── explicit only= guard ──
mkdir -p ~/guard-repo/files
echo "guarded" > ~/guard-repo/files/.guarded
cat > ~/guard-repo/dots.toml <<'TOML'
[meta]
version = 1

[[file]]
src = "files/.guarded"
dst = "~/.guarded"
only = ["darwin"]
TOML

dots --repo ~/guard-repo apply
assert_not_exists "darwin-only file skipped on linux" ~/.guarded

summary
