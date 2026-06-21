//go:build e2e

// Package-level end-to-end tests that build the real dots binary and drive it
// as a subprocess against a throwaway HOME and dotfiles repo. Unlike the
// in-process command tests, these exercise main(), the cobra wiring, on-disk
// config loading, and the real deploy path — the coverage the retired Python
// Docker e2e suite used to provide.
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

// dots runs the compiled binary with HOME pinned to home, returning combined
// output. It never fails the test itself so callers can assert on errors.
func dots(t *testing.T, home string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(dotsBin, args...)
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// newRepo scaffolds a dotfiles repo via `dots init` and returns its path.
func newRepo(t *testing.T, home string) string {
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

func TestE2E_Version(t *testing.T) {
	home := t.TempDir()
	out, err := dots(t, home, "--version")
	if err != nil {
		t.Fatalf("dots --version: %v\n%s", err, out)
	}
	// Built without ldflags, so the injected default applies.
	if !strings.Contains(out, "dots version dev") {
		t.Errorf("--version output = %q, want it to contain %q", out, "dots version dev")
	}
}

// TestE2E_ApplyDeploysSymlink is the headline path: init a repo, drop a managed
// dotfile, apply, and confirm the binary symlinked it into HOME pointing back at
// the repo source.
func TestE2E_ApplyDeploysSymlink(t *testing.T) {
	home := t.TempDir()
	repo := newRepo(t, home)

	src := filepath.Join(repo, "files", ".testrc")
	if err := os.WriteFile(src, []byte("# managed by dots\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := dots(t, home, "--repo", repo, "apply"); err != nil {
		t.Fatalf("dots apply: %v\n%s", err, out)
	}

	dst := filepath.Join(home, ".testrc")
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("expected ~/.testrc to exist: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("~/.testrc is not a symlink (mode %v)", fi.Mode())
	}
	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != src {
		t.Errorf("symlink target = %q, want %q", target, src)
	}
}

// TestE2E_ApplyIdempotent asserts invariant 3: a second apply changes nothing
// and still succeeds.
func TestE2E_ApplyIdempotent(t *testing.T) {
	home := t.TempDir()
	repo := newRepo(t, home)

	src := filepath.Join(repo, "files", ".testrc")
	if err := os.WriteFile(src, []byte("# managed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := dots(t, home, "--repo", repo, "apply"); err != nil {
		t.Fatalf("first apply: %v\n%s", err, out)
	}
	dst := filepath.Join(home, ".testrc")
	first, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("readlink after first apply: %v", err)
	}

	if out, err := dots(t, home, "--repo", repo, "apply"); err != nil {
		t.Fatalf("second apply: %v\n%s", err, out)
	}
	second, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("readlink after second apply: %v", err)
	}
	if first != second {
		t.Errorf("symlink changed between applies: %q -> %q", first, second)
	}
}

// TestE2E_DryRunNoSideEffects asserts invariant 5: --dry-run writes nothing.
func TestE2E_DryRunNoSideEffects(t *testing.T) {
	home := t.TempDir()
	repo := newRepo(t, home)

	src := filepath.Join(repo, "files", ".testrc")
	if err := os.WriteFile(src, []byte("# managed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := dots(t, home, "--repo", repo, "apply", "--dry-run"); err != nil {
		t.Fatalf("dots apply --dry-run: %v\n%s", err, out)
	}

	if _, err := os.Lstat(filepath.Join(home, ".testrc")); !os.IsNotExist(err) {
		t.Errorf("--dry-run created ~/.testrc (err=%v), want it absent", err)
	}
}

// TestE2E_StatusRuns confirms a read-only command works end-to-end against a
// freshly applied repo.
func TestE2E_StatusRuns(t *testing.T) {
	home := t.TempDir()
	repo := newRepo(t, home)

	src := filepath.Join(repo, "files", ".testrc")
	if err := os.WriteFile(src, []byte("# managed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := dots(t, home, "--repo", repo, "apply"); err != nil {
		t.Fatalf("apply: %v\n%s", err, out)
	}

	out, err := dots(t, home, "--repo", repo, "status")
	if err != nil {
		t.Fatalf("dots status: %v\n%s", err, out)
	}
	if !strings.Contains(out, ".testrc") {
		t.Errorf("status output did not mention the managed file:\n%s", out)
	}
}
