//go:build e2e

package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// ── git ───────────────────────────────────────────────────────────────────────

// TestE2E_GitManaged covers git init/show/uninit: managed.gitconfig is written,
// the [include] is added to ~/.gitconfig, and uninit removes it.
func TestE2E_GitManaged(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, `[meta]
version = 1

[git]
managed = true
name = "Test User"
email = "test@dots.dev"
editor = "nvim"
default_branch = "main"
`)

	mustDots(t, home, "--repo", repo, "git", "init")

	managed := readFile(t, filepath.Join(home, ".config", "dots", "git", "managed.gitconfig"))
	assertContains(t, "git name", managed, "Test User")
	assertContains(t, "git email", managed, "test@dots.dev")
	assertContains(t, "git editor", managed, "nvim")

	gitconfig := readFile(t, filepath.Join(home, ".gitconfig"))
	assertContains(t, "include added", gitconfig, "managed.gitconfig")

	out := mustDots(t, home, "--repo", repo, "git", "show")
	assertContains(t, "git show", out, "Test User")

	mustDots(t, home, "--repo", repo, "git", "uninit")
	gitconfig = readFile(t, filepath.Join(home, ".gitconfig"))
	assertNotContains(t, "include removed", gitconfig, "managed.gitconfig")
}

// TestE2E_GitManagedViaApply confirms apply orchestration wires in the git
// subsystem when [git] managed = true.
func TestE2E_GitManagedViaApply(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, `[meta]
version = 1

[git]
managed = true
name = "Apply User"
email = "apply@dots.dev"
`)

	mustDots(t, home, "--repo", repo, "apply")
	assertContains(t, "apply wrote managed.gitconfig",
		readFile(t, filepath.Join(home, ".config", "dots", "git", "managed.gitconfig")), "Apply User")
	assertContains(t, "apply added include",
		readFile(t, filepath.Join(home, ".gitconfig")), "managed.gitconfig")
}

// ── ssh ───────────────────────────────────────────────────────────────────────

// TestE2E_SSHManaged covers ssh init/show/uninit: the config fragment is
// generated, ~/.ssh is 0700, the Include is added, and uninit removes it.
func TestE2E_SSHManaged(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, `[meta]
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
`)

	mustDots(t, home, "--repo", repo, "ssh", "init")

	fragment := readFile(t, filepath.Join(home, ".config", "dots", "ssh", "config"))
	assertContains(t, "ssh host dev", fragment, "Host dev")
	assertContains(t, "ssh hostname", fragment, "HostName dev.example.com")
	assertContains(t, "ssh port", fragment, "Port 2222")
	assertContains(t, "ssh wildcard", fragment, "Host *.internal")

	sshConfig := readFile(t, filepath.Join(home, ".ssh", "config"))
	assertContains(t, "ssh include", sshConfig, "Include")
	assertMode(t, filepath.Join(home, ".ssh"), 0o700)

	out := mustDots(t, home, "--repo", repo, "ssh", "show")
	assertContains(t, "ssh show", out, "Host dev")

	mustDots(t, home, "--repo", repo, "ssh", "uninit")
	sshConfig = readFile(t, filepath.Join(home, ".ssh", "config"))
	assertNotContains(t, "ssh include removed", sshConfig, "Include ~/.config/dots/ssh/config")
}

// ── env ───────────────────────────────────────────────────────────────────────

// TestE2E_EnvShowAndCheck covers `env show` and the [[env.when]] conditional
// reporting in `env check`.
func TestE2E_EnvShowAndCheck(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, `[meta]
version = 1

[env]
EDITOR = "nvim"

[[env.when]]
key = "MAC_ONLY"
value = "1"
only = ["darwin"]
`)

	out := mustDots(t, home, "--repo", repo, "env", "show")
	assertContains(t, "env show EDITOR", out, `export EDITOR="nvim"`)

	out = mustDots(t, home, "--repo", repo, "env", "check")
	assertContains(t, "env check key", out, "MAC_ONLY")
	// On a non-darwin runner the darwin-only var is reported inactive.
	assertContains(t, "env check inactive marker", out, "✗")
}

// TestE2E_Profiles covers [profiles.<name>] env overrides flowing into the
// generated 010-env.sh.
func TestE2E_Profiles(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, `[meta]
version = 1

[env]
EDITOR = "nvim"

[shell]
managed = true

[profiles.work]
env.EDITOR = "code"
env.HTTP_PROXY = "http://proxy:8080"
`)

	envPath := filepath.Join(home, ".config", "dots", "shell.d", "010-env.sh")

	mustDots(t, home, "--repo", repo, "apply")
	assertContains(t, "default EDITOR", readFile(t, envPath), `export EDITOR="nvim"`)

	mustDots(t, home, "--repo", repo, "--profile", "work", "apply")
	env := readFile(t, envPath)
	assertContains(t, "work EDITOR", env, `export EDITOR="code"`)
	assertContains(t, "work proxy", env, `export HTTP_PROXY="http://proxy:8080"`)
}

// ── presets ───────────────────────────────────────────────────────────────────

// TestE2E_PresetsShowAndEject covers `presets show` and `presets eject`.
func TestE2E_PresetsShowAndEject(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)

	out := mustDots(t, home, "--repo", repo, "presets", "show", "tmux")
	assertContains(t, "tmux preset", out, "set -g prefix C-a")

	out = mustDots(t, home, "--repo", repo, "presets", "show", "fzf")
	assertContains(t, "fzf preset", out, "FZF_DEFAULT_OPTS")

	dest := filepath.Join(home, "ejected-tmux.conf")
	mustDots(t, home, "--repo", repo, "presets", "eject", "tmux", "--dest", dest)
	assertContains(t, "ejected file", readFile(t, dest), "set -g prefix C-a")

	// Unknown preset fails loudly.
	out, err := dots(t, home, "--repo", repo, "presets", "show", "bogus")
	if err == nil {
		t.Fatalf("unknown preset should fail; output:\n%s", out)
	}
	assertContains(t, "unknown preset error", out, "unknown preset")
}

// TestE2E_PresetsViaApply covers preset materialization during apply: tmux
// writes ~/.tmux.conf, and fzf (with shell managed) writes per-shell snippets.
func TestE2E_PresetsViaApply(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, `[meta]
version = 1

[shell]
managed = true

[presets]
fzf = true
tmux = true
`)

	mustDots(t, home, "--repo", repo, "apply")

	assertContains(t, "tmux.conf", readFile(t, filepath.Join(home, ".tmux.conf")), "set -g prefix C-a")
	shellD := filepath.Join(home, ".config", "dots", "shell.d")
	assertContains(t, "fzf zsh", readFile(t, filepath.Join(shellD, "030-fzf.zsh")), "FZF_DEFAULT_OPTS")
	assertContains(t, "fzf bash", readFile(t, filepath.Join(shellD, "030-fzf.bash")), "FZF_DEFAULT_OPTS")
}

// ── repos ─────────────────────────────────────────────────────────────────────

// TestE2E_ReposCloneStatusUpdate covers cloning a [[repo]] from a local source,
// reporting its status, and updating it. Uses a file:// URL so the GitHub
// shorthand expansion does not rewrite the path.
func TestE2E_ReposCloneStatusUpdate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	repo := scaffoldRepo(t)

	source := makeGitSource(t)
	writeToml(t, repo, `[meta]
version = 1

[[repo]]
name = "plugin"
repo = "file://`+source+`"
dst = "~/code/plugin"
`)

	// clone
	out := mustDots(t, home, "--repo", repo, "repos", "clone")
	assertContains(t, "clone reports", out, "plugin")
	cloned := filepath.Join(home, "code", "plugin")
	assertContains(t, "cloned content", readFile(t, filepath.Join(cloned, "README.md")), "hello")

	// status: ok
	out = mustDots(t, home, "--repo", repo, "repos", "status")
	assertContains(t, "status plugin", out, "plugin")
	assertContains(t, "status ok", out, "ok")

	// update: clean clone updates without error
	mustDots(t, home, "--repo", repo, "repos", "update")

	// clone again is idempotent (already present → no error).
	mustDots(t, home, "--repo", repo, "repos", "clone")
}

// makeGitSource creates a local git repository with one commit and returns its
// absolute path for use as a file:// clone source.
func makeGitSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	writeFile(t, filepath.Join(dir, "README.md"), "hello\n")
	run("add", "README.md")
	run("commit", "-q", "-m", "initial")
	return dir
}
