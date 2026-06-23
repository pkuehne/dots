//go:build e2e

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestE2E_Version asserts the binary reports its version (the default "dev"
// when built without ldflags — install.sh promises `dots --version`).
func TestE2E_Version(t *testing.T) {
	home := t.TempDir()
	out := mustDots(t, home, "--version")
	assertContains(t, "--version", out, "dots version dev")
}

// TestE2E_InitScaffold covers a fresh init and the guard against
// re-initializing over an existing dots.toml.
func TestE2E_InitScaffold(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(t.TempDir(), "fresh")

	out := mustDots(t, home, "init", dir)
	assertContains(t, "init success", out, "Initialized")
	for _, sub := range []string{"dots.toml", "files", "files.d", "shell"} {
		if _, err := os.Stat(filepath.Join(dir, sub)); err != nil {
			t.Errorf("init did not create %s: %v", sub, err)
		}
	}

	// Second init fails loudly rather than clobbering.
	out, err := dots(t, home, "init", dir)
	if err == nil {
		t.Fatalf("re-init should fail; output:\n%s", out)
	}
	assertContains(t, "re-init error", out, "already exists")
}

// TestE2E_Doctor runs the health check against a minimal repo and asserts it
// reports passing checks (warnings are advisory and do not fail the command).
func TestE2E_Doctor(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)

	out := mustDots(t, home, "--repo", repo, "doctor")
	assertContains(t, "doctor banner", out, "dots doctor")
	assertContains(t, "doctor checkmark", out, "✓")
	assertContains(t, "doctor finds toml", out, "dots.toml")
}

// TestE2E_Completion covers shell completion generation for each supported
// shell. completion is skipConfig, so it needs no repo.
func TestE2E_Completion(t *testing.T) {
	home := t.TempDir()
	for _, sh := range []string{"bash", "zsh", "fish"} {
		out := mustDots(t, home, "completion", sh)
		assertContains(t, sh+" completion", out, "dots")
	}
	if _, err := dots(t, home, "completion", "nonsense"); err == nil {
		t.Error("completion with invalid shell should fail")
	}
}

// TestE2E_Add covers adopting an existing HOME file into the repo, and that a
// second add is idempotent (no duplicate entry).
func TestE2E_Add(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)

	target := filepath.Join(home, ".myrc")
	writeFile(t, target, "rc content\n")

	out := mustDots(t, home, "--repo", repo, "add", target)
	assertContains(t, "add adopts", out, "Adopted")
	assertContains(t, "copied into repo", readFile(t, filepath.Join(repo, "files", ".myrc")), "rc content")
	assertContains(t, "toml entry", readFile(t, filepath.Join(repo, "dots.toml")), "[[file]]")

	out = mustDots(t, home, "--repo", repo, "add", target)
	assertContains(t, "add idempotent", out, "already managed")
}

// TestE2E_Migrate covers scanning HOME for unmanaged dotfiles and --write
// copying them into the repo with appended [[file]] entries.
func TestE2E_Migrate(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)
	writeFile(t, filepath.Join(home, ".vimrc"), "set nocompatible\n")

	out := mustDots(t, home, "--repo", repo, "migrate")
	assertContains(t, "migrate finds vimrc", out, ".vimrc")

	mustDots(t, home, "--repo", repo, "migrate", "--write")
	assertContains(t, "vimrc copied", readFile(t, filepath.Join(repo, "files", ".vimrc")), "set nocompatible")
	assertContains(t, "toml updated", readFile(t, filepath.Join(repo, "dots.toml")), ".vimrc")
}

// TestE2E_ToolsListCheck covers the tools subcommands against a configured tool
// and asserts an unknown tool name fails loudly.
func TestE2E_ToolsListCheck(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, `[meta]
version = 1

[[tool]]
name = "ripgrep"
desc = "fast grep"
check = "which rg"
tags = ["cli"]
`)

	out := mustDots(t, home, "--repo", repo, "tools", "list")
	assertContains(t, "tools list", out, "ripgrep")

	out = mustDots(t, home, "--repo", repo, "tools", "check")
	assertContains(t, "tools check name", out, "ripgrep")

	// Unknown tool fails loudly (likely typo).
	out, err := dots(t, home, "--repo", repo, "tools", "check", "bogus")
	if err == nil {
		t.Fatalf("unknown tool should fail; output:\n%s", out)
	}
	assertContains(t, "unknown tool error", out, "unknown tool")
}

// TestE2E_ApplyInstallsTools asserts `dots apply` runs the tool-install phase
// and reports it — including when a tool is already present — so apply is never
// silent about tools (#26). A "manual" method keeps the test offline.
func TestE2E_ApplyInstallsTools(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeToml(t, repo, `[meta]
version = 1

[[tool]]
name = "present-tool"
check = "true"

[[tool]]
name = "missing-tool"
check = "false"
[[tool.install]]
method = "manual"
note = "install it yourself"
`)

	out := mustDots(t, home, "--repo", repo, "apply")
	// The missing tool is acted on, and the summary always renders so an
	// all-present run is not silent.
	assertContains(t, "apply installs missing tool", out, "missing-tool")
	assertContains(t, "apply tools summary", out, "Tools:")
	assertContains(t, "apply tools present count", out, "1 already present")
}

// TestE2E_RepoResolutionError asserts a missing repo path produces a helpful
// error rather than a panic or traceback (invariant 6).
func TestE2E_RepoResolutionError(t *testing.T) {
	home := t.TempDir()
	out, err := dots(t, home, "--repo", filepath.Join(home, "does-not-exist"), "apply")
	if err == nil {
		t.Fatalf("apply against missing repo should fail; output:\n%s", out)
	}
	assertContains(t, "missing repo hint", out, "does not exist")
}
