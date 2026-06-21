//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pkuehne/dots/internal/shell"
)

// shellManagedToml returns a dots.toml enabling shell.managed with env and
// path, pointing the bootstrapper at files inside the test HOME.
const shellManagedToml = `[meta]
version = 1

[env]
EDITOR = "nvim"
LANG = "en_US.UTF-8"

[shell]
managed = true
dir = "~/.config/dots/shell.d"
path = ["~/.cargo/bin", "~/.local/bin"]
`

// TestE2E_ShellManaged covers the generated shell.d snippets and the
// bootstrapper insertion into the rc files.
func TestE2E_ShellManaged(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, shellManagedToml)
	// A user snippet in shell/ is deployed verbatim into shell.d.
	writeFile(t, filepath.Join(repo, "shell", "30-aliases.sh"), "# my aliases\n")

	mustDots(t, home, "--repo", repo, "apply")

	shellD := filepath.Join(home, ".config", "dots", "shell.d")

	// env snippet, sorted exports.
	env := readFile(t, filepath.Join(shellD, "010-env.sh"))
	assertContains(t, "env EDITOR", env, `export EDITOR="nvim"`)
	assertContains(t, "env LANG", env, `export LANG="en_US.UTF-8"`)
	assertContains(t, "env header", env, shell.GeneratedHeader)

	// path snippet, PATH guard.
	path := readFile(t, filepath.Join(shellD, "020-path.sh"))
	assertContains(t, "path cargo", path, ".cargo/bin")
	assertContains(t, "path local", path, ".local/bin")
	assertContains(t, "path guard", path, `case ":$PATH:"`)

	// user snippet deployed verbatim.
	assertContains(t, "user snippet", readFile(t, filepath.Join(shellD, "30-aliases.sh")), "# my aliases")

	// bootstrapper inserted into ~/.zshrc and ~/.bashrc.
	zshrc := readFile(t, filepath.Join(home, ".zshrc"))
	assertContains(t, "zshrc bootstrapper", zshrc, shell.MarkerStart)
	assertContains(t, "zshrc sources shell.d", zshrc, "shell.d")
	bashrc := readFile(t, filepath.Join(home, ".bashrc"))
	assertContains(t, "bashrc bootstrapper", bashrc, shell.MarkerStart)
}

// TestE2E_ShellBootstrapperIdempotent asserts the marker block is inserted
// exactly once across repeated applies (no duplication).
func TestE2E_ShellBootstrapperIdempotent(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, shellManagedToml)

	mustDots(t, home, "--repo", repo, "apply")
	mustDots(t, home, "--repo", repo, "apply")

	zshrc := readFile(t, filepath.Join(home, ".zshrc"))
	if n := countOccurrences(zshrc, shell.MarkerStart); n != 1 {
		t.Errorf("bootstrapper marker appears %d times in ~/.zshrc, want 1", n)
	}
}

// TestE2E_ShellShowAndClean covers `shell show` and `shell clean`. clean must
// remove a stale snippet while leaving the generated and user snippets intact.
func TestE2E_ShellShowAndClean(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, shellManagedToml)
	writeFile(t, filepath.Join(repo, "shell", "30-aliases.sh"), "# my aliases\n")
	mustDots(t, home, "--repo", repo, "apply")

	out := mustDots(t, home, "--repo", repo, "shell", "show")
	assertContains(t, "shell show env", out, "EDITOR")

	// Drop a stale snippet that no config produces.
	shellD := filepath.Join(home, ".config", "dots", "shell.d")
	stale := filepath.Join(shellD, "040-stale.sh")
	writeFile(t, stale, "# orphaned\n")

	out = mustDots(t, home, "--repo", repo, "shell", "clean")
	assertContains(t, "clean removes stale", out, "040-stale.sh")
	assertNotExists(t, stale)
	// Generated and user snippets survive.
	if _, err := readFileErr(filepath.Join(shellD, "010-env.sh")); err != nil {
		t.Errorf("clean removed generated 010-env.sh: %v", err)
	}
	if _, err := readFileErr(filepath.Join(shellD, "30-aliases.sh")); err != nil {
		t.Errorf("clean removed user 30-aliases.sh: %v", err)
	}
}

// TestE2E_ErrantZshrcWithShellManaged is the regression test for the real-world
// footgun that motivated this suite: a files/.zshrc deployed as a symlink to
// ~/.zshrc *while* shell.managed = true.
//
// The hazard: apply symlinks ~/.zshrc → repo/files/.zshrc, then writes the
// bootstrapper into ~/.zshrc — which, through the symlink, lands in the repo
// source. The bootstrapper sources every shell.d snippet. dots also wraps
// files/.zshrc into 099-custom.sh (itself a shell.d snippet). If 099-custom.sh
// kept the bootstrapper, sourcing it would re-source shell.d → infinite
// recursion. GenerateCustomSnippet strips the marker block to prevent exactly
// that. This test pins that behaviour.
func TestE2E_ErrantZshrcWithShellManaged(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, shellManagedToml)

	repoZshrc := filepath.Join(repo, "files", ".zshrc")
	writeFile(t, repoZshrc, "# my zshrc\nalias gco='git checkout'\n")

	mustDots(t, home, "--repo", repo, "apply")

	// ~/.zshrc is the deployed symlink into the repo, and carries the marker.
	homeZshrc := filepath.Join(home, ".zshrc")
	assertSymlinkTo(t, homeZshrc, repoZshrc)
	repoContent := readFile(t, repoZshrc)
	assertContains(t, "bootstrapper written through symlink", repoContent, shell.MarkerStart)

	// 099-custom.sh carries the user's aliases but NOT the marker block — the
	// recursion guard. This is the assertion that would have caught the bug.
	custom := readFile(t, filepath.Join(home, ".config", "dots", "shell.d", "099-custom.sh"))
	assertContains(t, "custom keeps aliases", custom, "alias gco='git checkout'")
	assertNotContains(t, "custom must not source shell.d (recursion)", custom, shell.MarkerStart)

	// A second apply must not corrupt or duplicate anything.
	mustDots(t, home, "--repo", repo, "apply")
	repoContent = readFile(t, repoZshrc)
	if n := countOccurrences(repoContent, shell.MarkerStart); n != 1 {
		t.Errorf("marker appears %d times in repo files/.zshrc after re-apply, want 1", n)
	}
	custom = readFile(t, filepath.Join(home, ".config", "dots", "shell.d", "099-custom.sh"))
	assertNotContains(t, "custom still has no marker after re-apply", custom, shell.MarkerStart)
}

// TestE2E_ShellSnippetPrefixWarning asserts a user snippet with a prefix outside
// the reserved user ranges produces a visible warning (but still deploys).
func TestE2E_ShellSnippetPrefixWarning(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, shellManagedToml)
	// 015 collides with the generated range (010/020) — should warn.
	writeFile(t, filepath.Join(repo, "shell", "015-bad.sh"), "# bad prefix\n")

	out := mustDots(t, home, "--repo", repo, "apply")
	assertContains(t, "prefix warning", out, "015-bad.sh")
	assertContains(t, "prefix warning text", out, "outside expected ranges")
}

// readFileErr is a thin wrapper used where a test wants to assert existence
// without fataling inside a helper.
func readFileErr(path string) (string, error) {
	data, err := os.ReadFile(path)
	return string(data), err
}
