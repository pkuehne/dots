// Package ssh manages the dots-owned SSH config fragment and the Include line
// inserted into ~/.ssh/config.
package ssh

import (
	"fmt"
	"os"
	"strings"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/fileutil"
	"github.com/pkuehne/dots/internal/shell"
)

// sshKeywordMap maps snake_case TOML keys to their SSH config keyword equivalents.
var sshKeywordMap = map[string]string{
	"host":                        "Host",
	"hostname":                    "HostName",
	"user":                        "User",
	"port":                        "Port",
	"identity_file":               "IdentityFile",
	"forward_agent":               "ForwardAgent",
	"proxy_jump":                  "ProxyJump",
	"proxy_command":               "ProxyCommand",
	"strict_host_key_checking":    "StrictHostKeyChecking",
	"user_known_hosts_file":       "UserKnownHostsFile",
	"server_alive_interval":       "ServerAliveInterval",
	"server_alive_count_max":      "ServerAliveCountMax",
	"compression":                 "Compression",
	"log_level":                   "LogLevel",
	"local_forward":               "LocalForward",
	"remote_forward":              "RemoteForward",
	"dynamic_forward":             "DynamicForward",
	"request_tty":                 "RequestTTY",
	"add_keys_to_agent":           "AddKeysToAgent",
	"identities_only":             "IdentitiesOnly",
	"certificate_file":            "CertificateFile",
	"preferred_authentications":   "PreferredAuthentications",
	"pubkey_accepted_algorithms":  "PubkeyAcceptedAlgorithms",
	"connect_timeout":             "ConnectTimeout",
}

// SnakeToSSHKeyword converts a snake_case key to its SSH config keyword.
// Known keys are looked up in the map; unknowns are TitleCased per word.
func SnakeToSSHKeyword(key string) string {
	if kw, ok := sshKeywordMap[key]; ok {
		return kw
	}
	parts := strings.Split(key, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

const sshIncludeLine = "Include ~/.config/dots/ssh/config"

// GenerateConfig returns the SSH config block for all active hosts.
func GenerateConfig(cfg config.Config, platform string) string {
	lines := []string{
		shell.GeneratedHeader,
		"# Source: dots.toml [[ssh.host]]",
		"# Regenerate: dots apply",
		"",
	}

	for _, host := range cfg.SSH.Hosts {
		if len(host.Only) > 0 && !sliceContains(host.Only, platform) {
			continue
		}
		lines = append(lines, "Host "+host.Host)
		for k, v := range host.Options {
			keyword := SnakeToSSHKeyword(k)
			lines = append(lines, "    "+keyword+" "+v)
		}
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n") + "\n"
}

// WriteManaged writes the SSH config fragment and inserts the Include line into ~/.ssh/config.
func WriteManaged(cfg config.Config, platform string, dryRun bool) error {
	outPath := fileutil.Expand("~/.config/dots/ssh/config")
	if !dryRun {
		if err := fileutil.EnsureParent(outPath); err != nil {
			return err
		}
		if err := os.WriteFile(outPath, []byte(GenerateConfig(cfg, platform)), 0o600); err != nil {
			return err
		}
	}

	sshDir := fileutil.Expand("~/.ssh")
	if !dryRun {
		if err := os.MkdirAll(sshDir, 0o700); err != nil {
			return err
		}
	}

	sshConfig := fileutil.Expand("~/.ssh/config")
	return insertSSHInclude(sshConfig, dryRun)
}

// ShowManaged prints the would-be SSH config fragment to stdout.
func ShowManaged(cfg config.Config, platform string) error {
	fmt.Print(GenerateConfig(cfg, platform))
	return nil
}

// Uninit removes the Include line from ~/.ssh/config.
func Uninit(cfg config.Config, dryRun bool) error {
	sshConfig := fileutil.Expand("~/.ssh/config")
	return removeSSHInclude(sshConfig, dryRun)
}

// insertSSHInclude prepends sshIncludeLine to path if not already present.
func insertSSHInclude(path string, dryRun bool) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	text := string(data)
	if strings.Contains(text, sshIncludeLine) {
		return nil
	}

	if dryRun {
		return nil
	}

	var newText string
	if text == "" {
		newText = sshIncludeLine + "\n"
	} else {
		newText = sshIncludeLine + "\n\n" + text
	}

	if err := fileutil.EnsureParent(path); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(newText), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

// removeSSHInclude removes the sshIncludeLine (and its trailing blank line) from path.
func removeSSHInclude(path string, dryRun bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	text := string(data)
	if !strings.Contains(text, sshIncludeLine) {
		return nil
	}

	// Remove the Include line and any immediately following blank line.
	newText := strings.Replace(text, sshIncludeLine+"\n\n", "", 1)
	if newText == text {
		newText = strings.Replace(text, sshIncludeLine+"\n", "", 1)
	}

	if dryRun {
		return nil
	}
	return os.WriteFile(path, []byte(newText), 0o600)
}

func sliceContains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
