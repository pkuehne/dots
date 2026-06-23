package config_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/errs"
)

// TestLoad_InlineToolInstall reproduces issue #29: an inline array-of-tables
// install method must be parsed exactly like the [[tool.install]] block form.
func TestLoad_InlineToolInstall(t *testing.T) {
	dir := writeToml(t, `
[[tool]]
name = "lazydocker"
check = "lazydocker --version"
install = [
  { method = "github", repo = "jesseduffield/lazydocker", asset = "lazydocker_*_Linux_{arch}.tar.gz", binary = "lazydocker" },
]
`)
	cfg, err := config.Load(dir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Tools) != 1 || len(cfg.Tools[0].Install) != 1 {
		t.Fatalf("expected 1 tool with 1 install method, got %+v", cfg.Tools)
	}
	if m := cfg.Tools[0].Install[0].Method; m != "github" {
		t.Errorf("install method: got %q, want %q", m, "github")
	}
}

// TestLoad_InlineEquivalentToBlock asserts the two TOML spellings of every
// array-of-tables key (tool, tool.install, file, repo, ssh.host, env.when)
// produce an identical Config. This directly encodes the invariant that the two
// forms are interchangeable, so any future helper that regresses one form fails
// here.
func TestLoad_InlineEquivalentToBlock(t *testing.T) {
	block := `
[[tool]]
name = "lazydocker"
check = "lazydocker --version"
[[tool.install]]
method = "github"
repo = "jesseduffield/lazydocker"

[[file]]
src = "files/.gitconfig"
dst = "~/.gitconfig"

[[repo]]
name = "tpm"
repo = "tmux-plugins/tpm"

[ssh]
[[ssh.host]]
host = "web"
user = "deploy"
port = 22

[env]
[[env.when]]
key = "EDITOR"
value = "vim"
`
	inline := `
tool = [
  { name = "lazydocker", check = "lazydocker --version", install = [ { method = "github", repo = "jesseduffield/lazydocker" } ] },
]
file = [ { src = "files/.gitconfig", dst = "~/.gitconfig" } ]
repo = [ { name = "tpm", repo = "tmux-plugins/tpm" } ]

[ssh]
host = [ { host = "web", user = "deploy", port = 22 } ]

[env]
when = [ { key = "EDITOR", value = "vim" } ]
`
	bcfg, err := config.Load(writeToml(t, block), "")
	if err != nil {
		t.Fatalf("block form failed: %v", err)
	}
	icfg, err := config.Load(writeToml(t, inline), "")
	if err != nil {
		t.Fatalf("inline form failed: %v", err)
	}
	// RepoRoot is the only field that legitimately differs (separate temp dirs).
	bcfg.RepoRoot, icfg.RepoRoot = "", ""
	if !reflect.DeepEqual(bcfg, icfg) {
		t.Errorf("inline and block forms produced different configs:\nblock=%+v\ninline=%+v", bcfg, icfg)
	}
}

func TestLoad_RejectsUnknownKey(t *testing.T) {
	dir := writeToml(t, `
[[tool]]
name = "ripgrep"
instal = [ { method = "github" } ]
`)
	_, err := config.Load(dir, "")
	if err == nil {
		t.Fatal("expected error for misspelled 'instal' key")
	}
	var ce *errs.ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("expected a ConfigError, got %T: %v", err, err)
	}
	if !strings.Contains(ce.Hint, "install") {
		t.Errorf("hint should suggest 'install', got: %q", ce.Hint)
	}
}

func TestLoad_RejectsWrongType(t *testing.T) {
	// 'name' is a scalar; an array must be rejected rather than silently dropped.
	dir := writeToml(t, `
[git]
name = ["Peter"]
`)
	if _, err := config.Load(dir, ""); err == nil {
		t.Fatal("expected error for git.name being an array")
	}
}

func TestLoad_RejectsNonTableArrayElement(t *testing.T) {
	// install must be an array of *tables*; a bare string element is malformed.
	dir := writeToml(t, `
[[tool]]
name = "ripgrep"
install = [ "github" ]
`)
	if _, err := config.Load(dir, ""); err == nil {
		t.Fatal("expected error for install element that is not a table")
	}
}

func TestLoad_RejectsUnknownKeyInProfile(t *testing.T) {
	// Typos inside an inactive profile must be caught too.
	dir := writeToml(t, `
[profiles.darwin.git]
naem = "Peter"
`)
	if _, err := config.Load(dir, ""); err == nil {
		t.Fatal("expected error for misspelled key inside [profiles.darwin.git]")
	}
}
