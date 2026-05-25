package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pkuehne/dots/internal/config"
)

// makeDir creates a temporary directory, optionally containing dots.toml or files/.
func makeDir(t *testing.T, hasToml, hasFilesDir bool) string {
	t.Helper()
	dir := t.TempDir()
	if hasToml {
		if err := os.WriteFile(filepath.Join(dir, "dots.toml"), []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if hasFilesDir {
		if err := os.Mkdir(filepath.Join(dir, "files"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestFindRepoRoot_ExplicitFlag(t *testing.T) {
	dir := makeDir(t, false, false)
	got, err := config.FindRepoRoot(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}

func TestFindRepoRoot_ExplicitFlagMissing(t *testing.T) {
	_, err := config.FindRepoRoot("/nonexistent/path/that/cannot/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent explicit path")
	}
}

func TestFindRepoRoot_EnvVar(t *testing.T) {
	dir := makeDir(t, false, false)
	t.Setenv("DOTS_REPO", dir)

	got, err := config.FindRepoRoot("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}

func TestFindRepoRoot_ExplicitTakesPrecedenceOverEnv(t *testing.T) {
	explicit := makeDir(t, false, false)
	env := makeDir(t, false, false)
	t.Setenv("DOTS_REPO", env)

	got, err := config.FindRepoRoot(explicit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != explicit {
		t.Errorf("got %q, want %q (explicit should win over env)", got, explicit)
	}
}

func TestFindRepoRoot_CwdWithToml(t *testing.T) {
	dir := makeDir(t, true, false)
	t.Chdir(dir)

	got, err := config.FindRepoRoot("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}

func TestFindRepoRoot_CwdWithFilesDir(t *testing.T) {
	dir := makeDir(t, false, true)
	t.Chdir(dir)

	got, err := config.FindRepoRoot("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
}

func TestFindRepoRoot_CwdNoMarkers(t *testing.T) {
	dir := makeDir(t, false, false)
	t.Chdir(dir)

	_, err := config.FindRepoRoot("")
	if err == nil {
		t.Fatal("expected error when cwd has no dots.toml or files/")
	}
}

func TestFindRepoRoot_DoesNotWalkUp(t *testing.T) {
	// Parent has dots.toml; child does not. Running from child should error.
	parent := makeDir(t, true, false)
	child := filepath.Join(parent, "subdir")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(child)

	_, err := config.FindRepoRoot("")
	if err == nil {
		t.Fatal("expected error: should not walk up to parent")
	}
}
