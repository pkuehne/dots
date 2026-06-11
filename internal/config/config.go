// Package config defines the Config type and loads dots.toml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/platform"
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

// supportedSchemaVersion is the highest [meta] version this build understands.
const supportedSchemaVersion = 1

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

	raw, err := parseDoc(path)
	if err != nil {
		return Config{}, err
	}

	merged := mergeProfiles(raw, platform.Detect(), platform.Hostname(), profile)

	if m, ok := merged["meta"].(map[string]any); ok {
		cfg.Meta = MetaConfig{
			Version:     int(intVal(m, "version", 1)),
			DefaultMode: str(m, "default_mode", "symlink"),
		}
	}
	if cfg.Meta.Version > supportedSchemaVersion {
		return Config{}, errs.NewConfig(
			fmt.Sprintf("dots.toml uses schema version %d, but this dots supports up to %d",
				cfg.Meta.Version, supportedSchemaVersion),
			"Upgrade dots, or lower [meta] version in dots.toml.",
		)
	}
	if v, ok := merged["vars"].(map[string]any); ok {
		cfg.Vars = v
	}
	if p, ok := raw["profiles"].(map[string]any); ok {
		cfg.Profiles = p
	}

	cfg.Env, err = parseEnv(merged)
	if err != nil {
		return Config{}, err
	}

	if s, ok := merged["shell"].(map[string]any); ok {
		cfg.Shell = ShellConfig{
			Managed: boolean(s, "managed", false),
			Login:   boolean(s, "login", false),
			Zshrc:   str(s, "zshrc", "~/.zshrc"),
			Bashrc:  str(s, "bashrc", "~/.bashrc"),
			Dir:     str(s, "dir", "~/.config/dots/shell.d"),
			Path:    strSlice(s, "path"),
		}
	}
	if g, ok := merged["git"].(map[string]any); ok {
		cfg.Git = GitConfig{
			Managed:       boolean(g, "managed", false),
			Name:          str(g, "name", ""),
			Email:         str(g, "email", ""),
			Editor:        str(g, "editor", ""),
			DefaultBranch: str(g, "default_branch", "main"),
			PullRebase:    boolean(g, "pull_rebase", false),
			SigningKey:     str(g, "signingkey", ""),
			Sign:          boolean(g, "sign", false),
		}
	}
	if s, ok := merged["ssh"].(map[string]any); ok {
		cfg.SSH = SSHConfig{
			Managed: boolean(s, "managed", false),
			Hosts:   parseSSHHosts(s),
		}
	}
	if t, ok := merged["tools"].(map[string]any); ok {
		cfg.ToolsConfig = ToolsConfig{
			BinDir: str(t, "bin_dir", "~/.local/bin"),
		}
	}

	for _, rt := range tableSlice(merged, "tool") {
		t, err := parseTool(rt)
		if err != nil {
			return Config{}, err
		}
		cfg.Tools = append(cfg.Tools, t)
	}
	for _, rf := range tableSlice(merged, "file") {
		f, err := parseFileEntry(rf)
		if err != nil {
			return Config{}, err
		}
		cfg.Files = append(cfg.Files, f)
	}
	for _, rr := range tableSlice(merged, "repo") {
		r, err := parseRepoEntry(rr)
		if err != nil {
			return Config{}, err
		}
		cfg.Repos = append(cfg.Repos, r)
	}

	if s, ok := merged["secrets"].(map[string]any); ok {
		cfg.Secrets = SecretsConfig{
			Recipient: str(s, "recipient", ""),
			Identity:  str(s, "identity", "~/.config/dots/key.txt"),
		}
	}
	if p, ok := merged["presets"].(map[string]any); ok {
		cfg.Presets = PresetsConfig{
			Fzf:  boolean(p, "fzf", false),
			Tmux: boolean(p, "tmux", false),
		}
	}

	return cfg, nil
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
