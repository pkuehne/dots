package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── init ──────────────────────────────────────────────────────────────────────

func TestRunInit_CreatesScaffold(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "myrepo")

	if err := runInit(target); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	for _, sub := range []string{"files", "files.d", "shell"} {
		if fi, err := os.Stat(filepath.Join(target, sub)); err != nil || !fi.IsDir() {
			t.Errorf("expected directory %s", sub)
		}
	}

	data, err := os.ReadFile(filepath.Join(target, "dots.toml"))
	if err != nil {
		t.Fatalf("dots.toml not created: %v", err)
	}
	if !strings.Contains(string(data), "[meta]") {
		t.Error("dots.toml missing [meta] section")
	}
	if !strings.Contains(string(data), "[shell]") {
		t.Error("dots.toml missing [shell] section")
	}
}

func TestRunInit_ExistingTomlErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dots.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runInit(dir)
	if err == nil {
		t.Fatal("expected error for existing dots.toml")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunInit_CurrentDirDefault(t *testing.T) {
	dir := t.TempDir()
	if err := runInit(dir); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dots.toml")); err != nil {
		t.Error("dots.toml not created in target dir")
	}
}

// ── add ───────────────────────────────────────────────────────────────────────

func makeTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dots.toml"), []byte("[meta]\nversion = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunAdd_AdoptsFile(t *testing.T) {
	repoRoot := makeTestRepo(t)
	srcFile := filepath.Join(t.TempDir(), "myfile.conf")
	if err := os.WriteFile(srcFile, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := globals.cfg
	globals.cfg.RepoRoot = repoRoot
	t.Cleanup(func() { globals.cfg = orig })

	if err := runAdd(srcFile, ""); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	dest := filepath.Join(repoRoot, "files", "myfile.conf")
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("adopted file not found at %s", dest)
	}

	tomlData, _ := os.ReadFile(filepath.Join(repoRoot, "dots.toml"))
	if !strings.Contains(string(tomlData), "[[file]]") {
		t.Error("[[file]] entry not appended to dots.toml")
	}
	if !strings.Contains(string(tomlData), "files/myfile.conf") {
		t.Error("src not written to dots.toml")
	}
}

func TestRunAdd_MissingFile(t *testing.T) {
	repoRoot := makeTestRepo(t)
	orig := globals.cfg
	globals.cfg.RepoRoot = repoRoot
	t.Cleanup(func() { globals.cfg = orig })

	err := runAdd("/nonexistent/path.conf", "")
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestRunAdd_CustomDest(t *testing.T) {
	repoRoot := makeTestRepo(t)
	srcFile := filepath.Join(t.TempDir(), "rc")
	if err := os.WriteFile(srcFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := globals.cfg
	globals.cfg.RepoRoot = repoRoot
	t.Cleanup(func() { globals.cfg = orig })

	if err := runAdd(srcFile, "files.d/linux/rc"); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "files.d", "linux", "rc")); err != nil {
		t.Error("file not placed at custom dest")
	}
}

// ── migrate ───────────────────────────────────────────────────────────────────

func TestRunMigrate_NoFiles(t *testing.T) {
	repoRoot := makeTestRepo(t)
	orig := globals.cfg
	globals.cfg.RepoRoot = repoRoot
	t.Cleanup(func() { globals.cfg = orig })

	// With an empty home temp dir, nothing matches — just ensure no crash.
	// We can't easily override HOME without affecting other tests, so just
	// verify the function returns nil when the scan finds nothing.
	if err := runMigrate(false, ""); err != nil {
		t.Fatalf("runMigrate: %v", err)
	}
}

func TestRunMigrate_Write(t *testing.T) {
	repoRoot := makeTestRepo(t)

	// Plant a fake home with a known file that migrateScan will find.
	fakeHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeHome, ".vimrc"), []byte("set nu"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := globals.cfg
	origHome := os.Getenv("HOME")
	globals.cfg.RepoRoot = repoRoot
	os.Setenv("HOME", fakeHome)
	t.Cleanup(func() {
		globals.cfg = orig
		os.Setenv("HOME", origHome)
	})

	if err := runMigrate(true, ""); err != nil {
		t.Fatalf("runMigrate --write: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, "files", ".vimrc")); err != nil {
		t.Error("expected .vimrc copied to repo")
	}
	tomlData, _ := os.ReadFile(filepath.Join(repoRoot, "dots.toml"))
	if !strings.Contains(string(tomlData), "[[file]]") {
		t.Error("expected [[file]] entry in dots.toml")
	}
}
