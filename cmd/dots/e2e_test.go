//go:build e2e

// Package-level end-to-end tests that build the real dots binary and drive it
// as a subprocess against a throwaway HOME and dotfiles repo. Unlike the
// in-process command tests, these exercise main(), the cobra wiring, on-disk
// config loading, and the real deploy path — the coverage the retired Python
// Docker e2e suite (tests/e2e/) used to provide, plus the edge cases that bit
// us in practice (e.g. an errant files/.zshrc with shell.managed = true).
//
// Layout:
//
//	e2e_test.go          TestMain, the dots() driver, and assertion helpers
//	e2e_files_test.go    file deployment: symlink/copy/mode/platform/profile/secret
//	e2e_shell_test.go    shell.managed: env/path/snippets/bootstrapper + edge cases
//	e2e_managed_test.go  git / ssh / env / presets / repos managed subsystems
//	e2e_cli_test.go      version/doctor/completion/init/add/migrate/error paths
//
// Run with: go test -tags e2e ./cmd/dots/...
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// dotsBin is the path to the binary compiled once in TestMain.
var dotsBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "dots-e2e-bin")
	if err != nil {
		panic("e2e: mkdtemp: " + err.Error())
	}
	dotsBin = filepath.Join(tmp, "dots")

	build := exec.Command("go", "build", "-o", dotsBin, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("e2e: build dots: " + err.Error())
	}

	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// ── driver ────────────────────────────────────────────────────────────────────

// dots runs the compiled binary with HOME pinned to home, returning combined
// output. It never fails the test itself so callers can assert on the error.
// HOME is pinned because every path dots touches (deploy targets, shell.d,
// ~/.gitconfig, …) is derived from it via os.UserHomeDir / fileutil.Expand.
func dots(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(dotsBin, args...)
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// mustDots runs dots and fails the test if the command errors, returning the
// combined output for further assertions.
func mustDots(t *testing.T, home string, args ...string) string {
	t.Helper()
	out, err := dots(t, home, args...)
	if err != nil {
		t.Fatalf("dots %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// ── repo scaffolding ──────────────────────────────────────────────────────────

// initRepo scaffolds a dotfiles repo via `dots init` and returns its path. This
// exercises the init command and yields the default dots.toml (shell/git
// unmanaged); tests needing other config overwrite it with writeToml.
func initRepo(t *testing.T, home string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "dotfiles")
	if out, err := dots(t, home, "init", repo); err != nil {
		t.Fatalf("dots init: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(repo, "dots.toml")); err != nil {
		t.Fatalf("init did not create dots.toml: %v", err)
	}
	return repo
}

// scaffoldRepo creates an empty repo tree (files/, files.d/, shell/) without a
// dots.toml, so a test can supply its own config via writeToml. Returns the
// repo root path.
func scaffoldRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, sub := range []string{"files", "files.d", "shell"} {
		if err := os.MkdirAll(filepath.Join(repo, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return repo
}

// writeToml writes dots.toml at the repo root.
func writeToml(t *testing.T, repo, content string) {
	t.Helper()
	writeFile(t, filepath.Join(repo, "dots.toml"), content)
}

// writeFile writes content to path, creating parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readFile returns the contents of path, failing the test if it cannot be read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// ── filesystem assertions ─────────────────────────────────────────────────────

// assertSymlinkTo asserts path is a symlink whose target equals want.
func assertSymlinkTo(t *testing.T, path, want string) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink (mode %v)", path, fi.Mode())
	}
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("readlink %s: %v", path, err)
	}
	if target != want {
		t.Errorf("symlink %s target = %q, want %q", path, target, want)
	}
}

// assertSymlink asserts path is a symlink (target unchecked).
func assertSymlink(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink (mode %v)", path, fi.Mode())
	}
}

// assertRegularFile asserts path exists and is a regular file (not a symlink).
func assertRegularFile(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s is a symlink, want a regular file", path)
	}
	if !fi.Mode().IsRegular() {
		t.Fatalf("%s is not a regular file (mode %v)", path, fi.Mode())
	}
}

// assertNotExists asserts nothing exists at path.
func assertNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("expected %s to be absent (err=%v)", path, err)
	}
}

// assertMode asserts the permission bits of path equal want.
func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := fi.Mode().Perm(); got != want {
		t.Errorf("%s mode = %o, want %o", path, got, want)
	}
}

// ── string assertions ─────────────────────────────────────────────────────────

func assertContains(t *testing.T, label, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("%s: expected to contain %q, got:\n%s", label, needle, haystack)
	}
}

func assertNotContains(t *testing.T, label, haystack, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Errorf("%s: expected NOT to contain %q, got:\n%s", label, needle, haystack)
	}
}

// countOccurrences returns how many times needle appears in s.
func countOccurrences(s, needle string) int {
	return strings.Count(s, needle)
}
