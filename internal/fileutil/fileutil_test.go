package fileutil_test

import (
	"os"
	"path/filepath"
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
