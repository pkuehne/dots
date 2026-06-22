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
	entries, err := discovery.Walk(cfg, []string{"linux"})
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
	entries, err := discovery.Walk(cfg, []string{"linux"})
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
	if e.Secret {
		t.Errorf("unexpected secret flag")
	}
}

func TestWalk_NestedFile(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := makeRepo(t, map[string]string{
		"files/.config/nvim/init.lua": "-- config",
	})
	entries, err := discovery.Walk(cfg, []string{"linux"})
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
		"files/.git/config":     "git stuff",
		"files/.gitconfig":      "[user]",
		"files/.DS_Store":       "junk",
		"files/real.txt":        "real",
		"files/backup.txt~":     "temp",
		"files/session.txt.swp": "swap",
	})
	entries, err := discovery.Walk(cfg, []string{"linux"})
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
	entries, err := discovery.Walk(cfg, []string{"linux"})
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

// .j2 files carry no special meaning: they are deployed verbatim like any other
// opaque file, suffix and all.
func TestWalk_J2FileTreatedAsOpaque(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := makeRepo(t, map[string]string{
		"files/.gitconfig.j2": "literal content",
	})
	entries, err := discovery.Walk(cfg, []string{"linux"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Dst != filepath.Join(home, ".gitconfig.j2") {
		t.Errorf("dst should keep .j2 suffix verbatim: got %q", e.Dst)
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

	entries, err := discovery.Walk(cfg, []string{"linux"})
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

	entries, err := discovery.Walk(cfg, []string{"linux"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
}

func find(entries []config.FileEntry, dst string) *config.FileEntry {
	for i := range entries {
		if entries[i].Dst == dst {
			return &entries[i]
		}
	}
	return nil
}

func countDst(entries []config.FileEntry, dst string) int {
	n := 0
	for _, e := range entries {
		if e.Dst == dst {
			n++
		}
	}
	return n
}

// TestWalk_PlatformDirScoped checks that files.d/<tag>/ trees are discovered
// only for active platform tags, and that each discovered entry carries
// Only={tag} so deploy re-filters it correctly.
func TestWalk_PlatformDirScoped(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := makeRepo(t, map[string]string{
		"files/.gitconfig":           "[core]",
		"files.d/linux/.config/sys":  "linux-only",
		"files.d/wsl/.config/wsl":    "wsl-only",
		"files.d/darwin/.config/mac": "mac-only",
	})

	entries, err := discovery.Walk(cfg, []string{"linux", "wsl"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if e := find(entries, filepath.Join(home, ".config/sys")); e == nil {
		t.Error("files.d/linux entry missing")
	} else if len(e.Only) != 1 || e.Only[0] != "linux" {
		t.Errorf("linux entry Only: got %v, want [linux]", e.Only)
	}
	if e := find(entries, filepath.Join(home, ".config/wsl")); e == nil {
		t.Error("files.d/wsl entry missing")
	} else if len(e.Only) != 1 || e.Only[0] != "wsl" {
		t.Errorf("wsl entry Only: got %v, want [wsl]", e.Only)
	}
	if find(entries, filepath.Join(home, ".config/mac")) != nil {
		t.Error("files.d/darwin entry must not appear for linux+wsl")
	}
}

// TestWalk_PlatformDirOverridesFiles checks that a files.d/<tag>/ entry
// overrides a same-destination files/ entry (later wins, ADR 004).
func TestWalk_PlatformDirOverridesFiles(t *testing.T) {
	home, _ := os.UserHomeDir()
	cfg := makeRepo(t, map[string]string{
		"files/.bashrc":       "base",
		"files.d/wsl/.bashrc": "wsl-override",
	})

	entries, err := discovery.Walk(cfg, []string{"linux", "wsl"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dst := filepath.Join(home, ".bashrc")
	if n := countDst(entries, dst); n != 1 {
		t.Fatalf("expected a single .bashrc entry, got %d", n)
	}
	e := find(entries, dst)
	if e.Src != filepath.Join("files.d", "wsl", ".bashrc") {
		t.Errorf("files.d should override files/: got src %q", e.Src)
	}
}
