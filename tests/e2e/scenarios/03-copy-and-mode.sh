#!/bin/bash
# Scenario: copy mode, file permissions, and sensitive directory handling.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 03: copy mode and permissions ==="

mkdir -p ~/perms-repo/files/.ssh
echo "machine example.com" > ~/perms-repo/files/.netrc
echo "Host *" > ~/perms-repo/files/.ssh/config
cat > ~/perms-repo/dots.toml <<'TOML'
[meta]
version = 1

[[file]]
src = "files/.netrc"
dst = "~/.netrc"
mode = "600"
link = false

[[file]]
src = "files/.ssh/config"
dst = "~/.ssh/config"
TOML

dots --repo ~/perms-repo apply

# .netrc should be a copy, not a symlink
assert_file_exists ".netrc deployed" ~/.netrc
assert_eq ".netrc is not a symlink" "false" "$([ -L ~/.netrc ] && echo true || echo false)"
assert_file_mode ".netrc has mode 600" "600" ~/.netrc

# .ssh directory should be 700
assert_file_mode ".ssh dir has mode 700" "700" ~/.ssh

# ── force copy via flag ──
mkdir -p ~/copy-repo/files
echo "force copied" > ~/copy-repo/files/.marker
echo -e '[meta]\nversion = 1' > ~/copy-repo/dots.toml

dots --repo ~/copy-repo apply --copy
assert_file_exists ".marker deployed" ~/.marker
assert_eq ".marker is a copy, not symlink" "false" "$([ -L ~/.marker ] && echo true || echo false)"
assert_eq ".marker content correct" "force copied" "$(cat ~/.marker)"

summary
