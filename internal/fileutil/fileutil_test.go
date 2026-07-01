package fileutil_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pkuehne/dots/internal/fileutil"
)

func TestExpand_Home(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := fileutil.Expand("~"); got != home {
		t.Errorf("Expand(~): got %q, want %q", got, home)
	}
	if got := fileutil.Expand("~/foo"); got != filepath.Join(home, "foo") {
		t.Errorf("Expand(~/foo): got %q, want %q", got, filepath.Join(home, "foo"))
	}
}

func TestExpand_EnvVar(t *testing.T) {
	t.Setenv("TESTVAR", "/some/path")
	if got := fileutil.Expand("$TESTVAR/bin"); got != "/some/path/bin" {
		t.Errorf("Expand($TESTVAR/bin): got %q", got)
	}
}

func TestExpand_Plain(t *testing.T) {
	if got := fileutil.Expand("/etc/hosts"); got != "/etc/hosts" {
		t.Errorf("Expand plain: got %q", got)
	}
}

func TestShouldSkip(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{".git", true},
		{".DS_Store", true},
		{"file.swp", true},
		{"file~", true},
		{".gitconfig", false},
		{"README.md", false},
		{".zshrc", false},
	}
	for _, tc := range cases {
		if got := fileutil.ShouldSkip(tc.name); got != tc.want {
			t.Errorf("ShouldSkip(%q): got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestEnsureParent_Normal(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "a", "b", "c", "file.txt")
	if err := fileutil.EnsureParent(path); err != nil {
		t.Fatalf("EnsureParent: %v", err)
	}
	info, err := os.Stat(filepath.Join(base, "a", "b", "c"))
	if err != nil || !info.IsDir() {
		t.Errorf("parent dir not created")
	}
}

func TestEnsureParent_SensitiveDir(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, ".ssh", "config")
	if err := fileutil.EnsureParent(path); err != nil {
		t.Fatalf("EnsureParent: %v", err)
	}
	info, err := os.Stat(filepath.Join(base, ".ssh"))
	if err != nil {
		t.Fatalf("dir not created: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf(".ssh mode: got %o, want 700", info.Mode().Perm())
	}
}

func TestEnsureParent_Idempotent(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "sub", "file.txt")
	if err := fileutil.EnsureParent(path); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := fileutil.EnsureParent(path); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestEnsureParent_ConcurrentSharedDir(t *testing.T) {
	// Two files sharing a not-yet-existing parent, created concurrently:
	// the EEXIST race in os.Mkdir must be tolerated (issue #45).
	base := t.TempDir()
	shared := filepath.Join(base, "config", "smug")
	paths := []string{
		filepath.Join(shared, "dashboard.yml"),
		filepath.Join(shared, "dotfiles.yml"),
	}

	const workers = 32
	errs := make(chan error, workers*len(paths))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		for _, p := range paths {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				errs <- fileutil.EnsureParent(p)
			}(p)
		}
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("EnsureParent under concurrency: %v", err)
		}
	}
	if info, err := os.Stat(shared); err != nil || !info.IsDir() {
		t.Fatalf("shared dir not created: info=%v err=%v", info, err)
	}
}

func TestEnsureParent_SymlinkedDir(t *testing.T) {
	base := t.TempDir()
	// A real directory, and a symlink pointing at it standing in for the
	// parent — e.g. ~/.config/nvim -> a home-manager/nix store directory.
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.EnsureParent(filepath.Join(link, "file.txt")); err != nil {
		t.Fatalf("EnsureParent through symlinked dir: %v", err)
	}
}

func TestEnsureParent_DanglingSymlink(t *testing.T) {
	base := t.TempDir()
	link := filepath.Join(base, "link")
	if err := os.Symlink(filepath.Join(base, "missing"), link); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.EnsureParent(filepath.Join(link, "file.txt")); err == nil {
		t.Fatal("expected error for dangling symlink parent, got nil")
	}
}

func TestBackup_File(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	bak, err := fileutil.Backup(src)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if bak != src+".dots-bak" {
		t.Errorf("backup path: got %q", bak)
	}
	data, _ := os.ReadFile(bak)
	if string(data) != "hello" {
		t.Errorf("backup content: got %q", data)
	}
}

func TestBackup_Symlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	bak, err := fileutil.Backup(link)
	if err != nil {
		t.Fatalf("Backup symlink: %v", err)
	}
	// Backup of a symlink should itself be a symlink.
	dest, err := os.Readlink(bak)
	if err != nil {
		t.Fatalf("backup is not a symlink: %v", err)
	}
	if dest != target {
		t.Errorf("symlink backup target: got %q, want %q", dest, target)
	}
}

func TestBackup_NonExistent(t *testing.T) {
	bak, err := fileutil.Backup("/nonexistent/path/xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bak != "" {
		t.Errorf("expected empty backup path, got %q", bak)
	}
}

func TestSHA256File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	h1, err := fileutil.SHA256File(path)
	if err != nil {
		t.Fatalf("SHA256File: %v", err)
	}
	// Same content → same hash.
	path2 := filepath.Join(dir, "f2.txt")
	if err := os.WriteFile(path2, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	h2, _ := fileutil.SHA256File(path2)
	if h1 != h2 {
		t.Errorf("identical content: hashes differ (%q vs %q)", h1, h2)
	}
	// Different content → different hash.
	path3 := filepath.Join(dir, "f3.txt")
	if err := os.WriteFile(path3, []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	h3, _ := fileutil.SHA256File(path3)
	if h1 == h3 {
		t.Errorf("different content: hashes should differ")
	}
}
