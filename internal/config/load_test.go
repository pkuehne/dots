package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pkuehne/dots/internal/config"
)

func writeToml(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dots.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoad_NoToml(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Meta.DefaultMode != "symlink" {
		t.Errorf("default mode: got %q, want %q", cfg.Meta.DefaultMode, "symlink")
	}
	if cfg.RepoRoot != dir {
		t.Errorf("RepoRoot: got %q, want %q", cfg.RepoRoot, dir)
	}
}

func TestLoad_Meta(t *testing.T) {
	dir := writeToml(t, `
[meta]
version = 1
default_mode = "copy"
`)
	cfg, err := config.Load(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Meta.Version != 1 {
		t.Errorf("version: got %d, want 1", cfg.Meta.Version)
	}
	if cfg.Meta.DefaultMode != "copy" {
		t.Errorf("default_mode: got %q, want %q", cfg.Meta.DefaultMode, "copy")
	}
}

func TestLoad_UnsupportedSchemaVersion(t *testing.T) {
	dir := writeToml(t, `
[meta]
version = 2
`)
	if _, err := config.Load(dir, ""); err == nil {
		t.Fatal("expected error for unsupported schema version")
	}
}

func TestLoad_ProfileMerge(t *testing.T) {
	dir := writeToml(t, `
[meta]
default_mode = "symlink"

[profiles.work.meta]
default_mode = "copy"

[profiles.work.git]
email = "me@work.com"
`)
	cfg, err := config.Load(dir, "work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Meta.DefaultMode != "copy" {
		t.Errorf("profile meta: got %q, want %q", cfg.Meta.DefaultMode, "copy")
	}
	if cfg.Git.Email != "me@work.com" {
		t.Errorf("profile git.email: got %q, want %q", cfg.Git.Email, "me@work.com")
	}
}

func TestLoad_EnvVars(t *testing.T) {
	dir := writeToml(t, `
[env]
EDITOR = "nvim"
PAGER = "less"
`)
	cfg, err := config.Load(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Env.Vars["EDITOR"] != "nvim" {
		t.Errorf("EDITOR: got %q, want %q", cfg.Env.Vars["EDITOR"], "nvim")
	}
	if cfg.Env.Vars["PAGER"] != "less" {
		t.Errorf("PAGER: got %q, want %q", cfg.Env.Vars["PAGER"], "less")
	}
}

func TestLoad_EnvWhen(t *testing.T) {
	dir := writeToml(t, `
[[env.when]]
key = "EDITOR"
value = "nvim"
if_tool = "nvim"
`)
	cfg, err := config.Load(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Env.When) != 1 {
		t.Fatalf("[[env.when]]: got %d entries, want 1", len(cfg.Env.When))
	}
	ew := cfg.Env.When[0]
	if ew.Key != "EDITOR" || ew.Value != "nvim" || ew.IfTool != "nvim" {
		t.Errorf("env.when: got %+v", ew)
	}
}

func TestLoad_EnvError_PATH(t *testing.T) {
	dir := writeToml(t, `
[env]
PATH = "/usr/local/bin"
`)
	_, err := config.Load(dir, "")
	if err == nil {
		t.Fatal("expected error for PATH in [env]")
	}
}

func TestLoad_EnvError_Lowercase(t *testing.T) {
	dir := writeToml(t, `
[env]
editor = "nvim"
`)
	_, err := config.Load(dir, "")
	if err == nil {
		t.Fatal("expected error for lowercase env key")
	}
}

func TestLoad_Files(t *testing.T) {
	dir := writeToml(t, `
[[file]]
src = "files/.gitconfig"
dst = "~/.gitconfig"

[[file]]
src = "files/.zshrc"
dst = "~/.zshrc"
template = true
`)
	cfg, err := config.Load(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Files) != 2 {
		t.Fatalf("files: got %d, want 2", len(cfg.Files))
	}
	if cfg.Files[1].Template != true {
		t.Errorf("files[1].template: got false, want true")
	}
}

func TestLoad_FileMissingSrc(t *testing.T) {
	dir := writeToml(t, `
[[file]]
dst = "~/.gitconfig"
`)
	_, err := config.Load(dir, "")
	if err == nil {
		t.Fatal("expected error for [[file]] missing src")
	}
}

func TestLoad_FileMissingDst(t *testing.T) {
	dir := writeToml(t, `
[[file]]
src = "files/.gitconfig"
`)
	_, err := config.Load(dir, "")
	if err == nil {
		t.Fatal("expected error for [[file]] missing dst")
	}
}

func TestLoad_Tools(t *testing.T) {
	dir := writeToml(t, `
[[tool]]
name = "ripgrep"
tags = ["cli", "search"]

[[tool.install]]
method = "github"
repo = "BurntSushi/ripgrep"
asset = "ripgrep-{version}-x86_64-unknown-linux-musl.tar.gz"
binary = "rg"
version = "14.1.1"
`)
	cfg, err := config.Load(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Tools) != 1 {
		t.Fatalf("tools: got %d, want 1", len(cfg.Tools))
	}
	tool := cfg.Tools[0]
	if tool.Name != "ripgrep" {
		t.Errorf("tool.name: got %q", tool.Name)
	}
	if tool.Check != "which ripgrep" {
		t.Errorf("tool.check default: got %q, want %q", tool.Check, "which ripgrep")
	}
	if len(tool.Install) != 1 || tool.Install[0].Method != "github" {
		t.Errorf("tool.install: got %+v", tool.Install)
	}
}

func TestLoad_ToolMissingName(t *testing.T) {
	dir := writeToml(t, `
[[tool]]
desc = "some tool"
`)
	_, err := config.Load(dir, "")
	if err == nil {
		t.Fatal("expected error for [[tool]] missing name")
	}
}

func TestLoad_SSHHosts(t *testing.T) {
	dir := writeToml(t, `
[ssh]
managed = true

[[ssh.host]]
host = "myserver"
HostName = "192.168.1.10"
User = "peter"
Port = "2222"
`)
	cfg, err := config.Load(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.SSH.Managed {
		t.Error("ssh.managed: got false, want true")
	}
	if len(cfg.SSH.Hosts) != 1 {
		t.Fatalf("ssh.hosts: got %d, want 1", len(cfg.SSH.Hosts))
	}
	h := cfg.SSH.Hosts[0]
	if h.Host != "myserver" {
		t.Errorf("host: got %q", h.Host)
	}
	if h.Options["HostName"] != "192.168.1.10" {
		t.Errorf("HostName option: got %q", h.Options["HostName"])
	}
	if h.Options["User"] != "peter" {
		t.Errorf("User option: got %q", h.Options["User"])
	}
}

// TestLoad_SSHHostScalarOptions guards against the regression where non-string
// SSH option values (a native TOML integer port, a bool) were silently dropped
// instead of being coerced to strings.
func TestLoad_SSHHostScalarOptions(t *testing.T) {
	dir := writeToml(t, `
[ssh]
managed = true

[[ssh.host]]
host = "dev"
Port = 2222
ForwardAgent = true
Compression = false
`)
	cfg, err := config.Load(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.SSH.Hosts) != 1 {
		t.Fatalf("ssh.hosts: got %d, want 1", len(cfg.SSH.Hosts))
	}
	opts := cfg.SSH.Hosts[0].Options
	if opts["Port"] != "2222" {
		t.Errorf("Port option: got %q, want %q (integer must not be dropped)", opts["Port"], "2222")
	}
	if opts["ForwardAgent"] != "yes" {
		t.Errorf("ForwardAgent option: got %q, want %q", opts["ForwardAgent"], "yes")
	}
	if opts["Compression"] != "no" {
		t.Errorf("Compression option: got %q, want %q", opts["Compression"], "no")
	}
}

func TestLoad_Repos(t *testing.T) {
	dir := writeToml(t, `
[[repo]]
name = "tpm"
repo = "https://github.com/tmux-plugins/tpm"
dst = "~/.tmux/plugins/tpm"
shallow = true
`)
	cfg, err := config.Load(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("repos: got %d, want 1", len(cfg.Repos))
	}
	r := cfg.Repos[0]
	if r.Name != "tpm" || !r.Shallow {
		t.Errorf("repo: got %+v", r)
	}
}

func TestLoad_RepoMissingName(t *testing.T) {
	dir := writeToml(t, `
[[repo]]
repo = "https://github.com/foo/bar"
`)
	_, err := config.Load(dir, "")
	if err == nil {
		t.Fatal("expected error for [[repo]] missing name")
	}
}

func TestLoad_InvalidToml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dots.toml"), []byte("[[[\ninot valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(dir, "")
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}
