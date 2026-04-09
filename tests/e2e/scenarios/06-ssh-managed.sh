#!/bin/bash
# Scenario: SSH managed mode.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

echo "=== 06: SSH managed mode ==="

mkdir -p ~/ssh-repo/files
cat > ~/ssh-repo/dots.toml <<'TOML'
[meta]
version = 1

[ssh]
managed = true

[[ssh.host]]
host = "dev"
hostname = "dev.example.com"
user = "deploy"
port = 2222

[[ssh.host]]
host = "*.internal"
user = "admin"
identity_file = "~/.ssh/id_ed25519"
TOML

# ── ssh init ──
dots --repo ~/ssh-repo ssh init
assert_file_exists "SSH config generated" ~/.config/dots/ssh/config
sc=$(cat ~/.config/dots/ssh/config)
assert_contains "has dev host" "Host dev" "$sc"
assert_contains "has hostname" "HostName dev.example.com" "$sc"
assert_contains "has port" "Port 2222" "$sc"
assert_contains "has wildcard host" "Host *.internal" "$sc"

# Check include was added to ~/.ssh/config
assert_file_exists "~/.ssh/config exists" ~/.ssh/config
sshconfig=$(cat ~/.ssh/config)
assert_contains "include added" "dots" "$sshconfig"
assert_file_mode ".ssh dir is 700" "700" ~/.ssh

# ── ssh show ──
output=$(dots --repo ~/ssh-repo ssh show 2>&1)
assert_contains "ssh show has host" "Host dev" "$output"

# ── ssh uninit ──
dots --repo ~/ssh-repo ssh uninit
sshconfig_after=$(cat ~/.ssh/config)
assert_eq "include removed" "false" "$(echo "$sshconfig_after" | grep -q 'dots' && echo true || echo false)"

summary
