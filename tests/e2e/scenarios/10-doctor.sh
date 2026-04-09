#!/bin/bash
# Scenario: dots doctor health check.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 10: doctor ==="

mkdir -p ~/doc-repo/files
cat > ~/doc-repo/dots.toml <<'TOML'
[meta]
version = 1
TOML

output=$(dots --repo ~/doc-repo doctor 2>&1 || true)
assert_contains "doctor checks python" "Python" "$output"
assert_contains "doctor checks dots.toml" "dots.toml" "$output"
assert_contains "doctor shows checkmark" "✓" "$output"

summary
