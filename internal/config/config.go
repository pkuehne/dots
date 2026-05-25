// Package config defines the Config type and loads dots.toml.
package config

import (
	"os"
	"path/filepath"

	"github.com/pkuehne/dots/internal/errs"
)

// ── Structs ──────────────────────────────────────────────────────────────────

// MetaConfig holds [meta] settings.
type MetaConfig struct {
	Version     int    `toml:"version"`
	DefaultMode string `toml:"default_mode"`
}

// ShellConfig holds [shell] settings.
type ShellConfig struct {
	Managed bool     `toml:"managed"`
	Login   bool     `toml:"login"`
	Zshrc   string   `toml:"zshrc"`
	Bashrc  string   `toml:"bashrc"`
	Dir     string   `toml:"dir"`
	Path    []string `toml:"path"`
}

// GitConfig holds [git] settings.
type GitConfig struct {
	Managed       bool   `toml:"managed"`
	Name          string `toml:"name"`
	Email         string `toml:"email"`
	Editor        string `toml:"editor"`
	DefaultBranch string `toml:"default_branch"`
	PullRebase    bool   `toml:"pull_rebase"`
	SigningKey     string `toml:"signingkey"`
	Sign          bool   `toml:"sign"`
}

// SSHConfig holds [ssh] settings.
type SSHConfig struct {
	Managed bool      `toml:"managed"`
	Hosts   []SSHHost `toml:"host"`
}

// SSHHost holds one [[ssh.host]] entry.
// The SSH options (HostName, User, Port, etc.) are captured in Options.
// They cannot be decoded by the TOML library automatically because they are
// arbitrary — we decode them from the raw map after the main pass.
type SSHHost struct {
	Host    string            `toml:"host"`
	Only    []string          `toml:"only"`
	Options map[string]string `toml:"-"`
}

// ToolsConfig holds [tools] settings.
type ToolsConfig struct {
	BinDir string `toml:"bin_dir"`
}

// ToolShell holds [tool.shell] settings within a [[tool]] entry.
type ToolShell struct {
	Env  map[string]string `toml:"env"`
	Init string            `toml:"init"`
	Path []string          `toml:"path"`
}

// ToolGit holds [tool.git] settings within a [[tool]] entry.
type ToolGit struct {
	Pager bool `toml:"pager"`
	Diff  bool `toml:"diff"`
}

// ToolInstall holds one [[tool.install]] entry.
type ToolInstall struct {
	Method     string            `toml:"method"`
	Package    string            `toml:"package"`
	Repo       string            `toml:"repo"`
	Asset      string            `toml:"asset"`
	Binary     string            `toml:"binary"`
	BinaryPath string            `toml:"binary_path"`
	Version    string            `toml:"version"`
	Script     string            `toml:"script"`
	Note       string            `toml:"note"`
	Only       []string          `toml:"only"`
	ArchMap    map[string]string `toml:"arch_map"`
}

// Tool holds one [[tool]] entry.
type Tool struct {
	Name    string        `toml:"name"`
	Desc    string        `toml:"desc"`
	Check   string        `toml:"check"`
	Tags    []string      `toml:"tags"`
	Only    []string      `toml:"only"`
	Profile string        `toml:"profile"`
	Install []ToolInstall `toml:"install"`
	Shell   ToolShell     `toml:"shell"`
	Git     ToolGit       `toml:"git"`
}

// FileEntry holds one [[file]] entry.
type FileEntry struct {
	Src      string   `toml:"src"`
	Dst      string   `toml:"dst"`
	Only     []string `toml:"only"`
	Profile  string   `toml:"profile"`
	Template bool     `toml:"template"`
	Secret   bool     `toml:"secret"`
	Mode     string   `toml:"mode"`
	Link     *bool    `toml:"link"` // nil means "use default_mode"
}

// RepoEntry holds one [[repo]] entry.
type RepoEntry struct {
	Name      string   `toml:"name"`
	Repo      string   `toml:"repo"`
	Dst       string   `toml:"dst"`
	Shallow   bool     `toml:"shallow"`
	Ref       string   `toml:"ref"`
	OnInstall string   `toml:"on_install"`
	OnUpdate  string   `toml:"on_update"`
	Only      []string `toml:"only"`
	Profile   string   `toml:"profile"`
}

// EnvWhen holds one [[env.when]] conditional entry.
type EnvWhen struct {
	Key    string   `toml:"key"`
	Value  string   `toml:"value"`
	IfTool string   `toml:"if_tool"`
	Only   []string `toml:"only"`
}

// EnvConfig holds the parsed [env] section.
// The [env] section mixes plain KEY=value pairs with [[env.when]] entries,
// so it cannot be decoded directly by the TOML library — see parse.go.
type EnvConfig struct {
	Vars map[string]string
	When []EnvWhen
}

// SecretsConfig holds [secrets] settings.
type SecretsConfig struct {
	Recipient string `toml:"recipient"`
	Identity  string `toml:"identity"`
}

// PresetsConfig holds [presets] settings.
type PresetsConfig struct {
	Fzf  bool `toml:"fzf"`
	Tmux bool `toml:"tmux"`
}

// Config is the fully parsed and profile-merged dots.toml.
type Config struct {
	Meta          MetaConfig
	Vars          map[string]any
	Profiles      map[string]any
	Env           EnvConfig
	Shell         ShellConfig
	Git           GitConfig
	SSH           SSHConfig
	ToolsConfig   ToolsConfig
	Tools         []Tool
	Files         []FileEntry
	Repos         []RepoEntry
	Secrets       SecretsConfig
	Presets       PresetsConfig
	RepoRoot      string
	ActiveProfile string
}

// ── Defaults ─────────────────────────────────────────────────────────────────

// defaults returns a Config with all zero-config defaults applied.
func defaults() Config {
	return Config{
		Meta:        MetaConfig{Version: 1, DefaultMode: "symlink"},
		Shell:       ShellConfig{Zshrc: "~/.zshrc", Bashrc: "~/.bashrc", Dir: "~/.config/dots/shell.d"},
		Git:         GitConfig{DefaultBranch: "main"},
		ToolsConfig: ToolsConfig{BinDir: "~/.local/bin"},
		Secrets:     SecretsConfig{Identity: "~/.config/dots/key.txt"},
		Vars:        make(map[string]any),
		Profiles:    make(map[string]any),
		Env:         EnvConfig{Vars: make(map[string]string)},
	}
}

// ── Loader ───────────────────────────────────────────────────────────────────

// Load parses dots.toml from repoRoot, applies profile layering, and returns a
// fully populated Config. If dots.toml does not exist, returns defaults with
// RepoRoot set — this is the zero-config mirror mode.
func Load(repoRoot, profile string) (Config, error) {
	cfg := defaults()
	cfg.RepoRoot = repoRoot
	cfg.ActiveProfile = profile

	path := filepath.Join(repoRoot, "dots.toml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	panic("Load: not yet implemented — see parse.go")
}

// FindRepoRoot resolves the dotfiles repository root using this precedence:
//  1. explicit path (--repo flag)
//  2. $DOTS_REPO environment variable
//  3. current directory, if it contains dots.toml or a files/ subdirectory
//
// It does not walk up the tree — dots must be run from inside the repo, or the
// location must be given explicitly.
func FindRepoRoot(explicit string) (string, error) {
	if explicit != "" {
		return resolveRepoAt(explicit)
	}
	if env := os.Getenv("DOTS_REPO"); env != "" {
		return resolveRepoAt(env)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", errs.NewConfig("cannot determine current directory", err.Error())
	}
	if isRepoRoot(cwd) {
		return cwd, nil
	}
	return "", errs.NewConfig(
		"no dots repository found in current directory",
		"Run dots from inside your dotfiles repo, or use one of:\n"+
			"  dots --repo ~/dotfiles apply\n"+
			"  export DOTS_REPO=~/dotfiles",
	)
}

// resolveRepoAt resolves and validates an explicit repo path.
func resolveRepoAt(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", errs.NewConfig("invalid repo path: "+path, err.Error())
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		return "", errs.NewConfig(
			"repo path does not exist: "+abs,
			"Check that the directory exists and the path is correct.",
		)
	}
	return abs, nil
}

// isRepoRoot reports whether dir looks like a dots repository root.
// A directory qualifies if it contains dots.toml or a files/ subdirectory.
func isRepoRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "dots.toml")); err == nil {
		return true
	}
	if info, err := os.Stat(filepath.Join(dir, "files")); err == nil && info.IsDir() {
		return true
	}
	return false
}
