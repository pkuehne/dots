package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/discovery"
)

func makeRepo(t *testing.T, files map[string]string) config.Config {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return config.Config{RepoRoot: dir}
}

func TestWalk_EmptyRepo(t *testing.T) {
	cfg := makeRepo(t, nil)
	entries, err := discovery.Walk(cfg, "linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestWalk_SimpleFile(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := makeRepo(t, map[string]string{
		"files/.gitconfig": "[user]\n  name = test",
	})
	entries, err := discovery.Walk(cfg, "linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Src != "files/.gitconfig" {
		t.Errorf("src: got %q, want %q", e.Src, "files/.gitconfig")
	}
	if e.Dst != filepath.Join(home, ".gitconfig") {
		t.Errorf("dst: got %q, want %q", e.Dst, filepath.Join(home, ".gitconfig"))
	}
	if e.Template || e.Secret {
		t.Errorf("unexpected template/secret flags")
	}
}

func TestWalk_NestedFile(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := makeRepo(t, map[string]string{
		"files/.config/nvim/init.lua": "-- config",
	})
	entries, err := discovery.Walk(cfg, "linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Dst != filepath.Join(home, ".config/nvim/init.lua") {
		t.Errorf("dst: got %q", entries[0].Dst)
	}
}

func TestWalk_SkipsGitDir(t *testing.T) {
	cfg := makeRepo(t, map[string]string{
		"files/.git/config":    "git stuff",
		"files/.gitconfig":     "[user]",
		"files/.DS_Store":      "junk",
		"files/real.txt":       "real",
		"files/backup.txt~":    "temp",
		"files/session.txt.swp": "swap",
	})
	entries, err := discovery.Walk(cfg, "linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range entries {
		base := filepath.Base(e.Src)
		if base == ".DS_Store" || base == "config" || base == "backup.txt~" || base == "session.txt.swp" {
			t.Errorf("should have skipped %q", e.Src)
		}
	}
	if len(entries) != 2 { // .gitconfig + real.txt
		t.Errorf("got %d entries, want 2: %v", len(entries), entries)
	}
}

func TestWalk_AgeFileFlagged(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := makeRepo(t, map[string]string{
		"files/.ssh/id_rsa.age": "encrypted",
	})
	entries, err := discovery.Walk(cfg, "linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if !e.Secret {
		t.Errorf("expected Secret=true for .age file")
	}
	if e.Dst != filepath.Join(home, ".ssh/id_rsa") {
		t.Errorf("dst should strip .age: got %q", e.Dst)
	}
}

func TestWalk_J2FileFlagged(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := makeRepo(t, map[string]string{
		"files/.gitconfig.j2": "template",
	})
	entries, err := discovery.Walk(cfg, "linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if !e.Template {
		t.Errorf("expected Template=true for .j2 file")
	}
	if e.Dst != filepath.Join(home, ".gitconfig") {
		t.Errorf("dst should strip .j2: got %q", e.Dst)
	}
}

func TestWalk_ExplicitOverridesDiscovered(t *testing.T) {
	cfg := makeRepo(t, map[string]string{
		"files/.gitconfig": "[user]",
	})
	explicit := config.FileEntry{
		Src:  "files/.gitconfig",
		Dst:  "/custom/path/.gitconfig",
		Only: []string{"linux"},
	}
	cfg.Files = []config.FileEntry{explicit}

	entries, err := discovery.Walk(cfg, "linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Dst != "/custom/path/.gitconfig" {
		t.Errorf("explicit should override discovered: got %q", entries[0].Dst)
	}
}

func TestWalk_ExplicitAppended(t *testing.T) {
	cfg := makeRepo(t, map[string]string{
		"files/.gitconfig": "[user]",
	})
	extra := config.FileEntry{Src: "extra/file", Dst: "~/.extra"}
	cfg.Files = []config.FileEntry{extra}

	entries, err := discovery.Walk(cfg, "linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
}
