package deploy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/deploy"
)

// makeRepo creates a repo with a files/ dir containing the given files and
// returns the repo root and a base Options for tests to extend.
func makeRepo(t *testing.T, files map[string]string) (string, deploy.Options) {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir() // fake home so $HOME validation passes
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, deploy.Options{RepoRoot: root, DefaultMode: "symlink", HomeDir: home}
}

func homeDst(t *testing.T, opts deploy.Options, rel string) string {
	t.Helper()
	return filepath.Join(opts.HomeDir, rel)
}

func entry(src, dst string) config.FileEntry {
	return config.FileEntry{Src: src, Dst: dst}
}

// ── Symlink ───────────────────────────────────────────────────────────────────

func TestApply_Symlink(t *testing.T) {
	root, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	dst := homeDst(t, opts, ".gitconfig")

	r := deploy.Apply(entry("files/.gitconfig", dst), opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if r.Action != "linked" {
		t.Errorf("action: got %q, want %q", r.Action, "linked")
	}
	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("dst is not a symlink: %v", err)
	}
	if target != filepath.Join(root, "files/.gitconfig") {
		t.Errorf("symlink target: got %q", target)
	}
}

func TestApply_SymlinkIdempotent(t *testing.T) {
	root, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	_ = root
	dst := homeDst(t, opts, ".gitconfig")

	deploy.Apply(entry("files/.gitconfig", dst), opts)
	r := deploy.Apply(entry("files/.gitconfig", dst), opts)
	if r.Action != "unchanged" {
		t.Errorf("second apply action: got %q, want %q", r.Action, "unchanged")
	}
}

func TestApply_SymlinkReplacesStale(t *testing.T) {
	root, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	dstDir := opts.HomeDir
	dst := filepath.Join(dstDir, ".gitconfig")
	other := filepath.Join(dstDir, "other")
	os.WriteFile(other, []byte("x"), 0o644)
	os.Symlink(other, dst) // stale symlink

	r := deploy.Apply(entry("files/.gitconfig", dst), opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if r.Action != "linked" {
		t.Errorf("action: got %q, want %q", r.Action, "linked")
	}
	// Original stale link backed up.
	if _, err := os.Lstat(dst + ".dots-bak"); err != nil {
		t.Errorf("backup not created: %v", err)
	}
	_ = root
}

func TestApply_SymlinkReplacesFile(t *testing.T) {
	root, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	_ = root
	dst := homeDst(t, opts, ".gitconfig")
	os.WriteFile(dst, []byte("old content"), 0o644)

	r := deploy.Apply(entry("files/.gitconfig", dst), opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if r.Action != "linked" {
		t.Errorf("action: got %q, want %q", r.Action, "linked")
	}
}

// ── Copy ──────────────────────────────────────────────────────────────────────

func TestApply_Copy(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	opts.DefaultMode = "copy"
	dst := homeDst(t, opts, ".gitconfig")

	r := deploy.Apply(entry("files/.gitconfig", dst), opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if r.Action != "copied" {
		t.Errorf("action: got %q, want %q", r.Action, "copied")
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "[user]" {
		t.Errorf("content: got %q", data)
	}
}

func TestApply_CopyIdempotent(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	opts.DefaultMode = "copy"
	dst := homeDst(t, opts, ".gitconfig")

	deploy.Apply(entry("files/.gitconfig", dst), opts)
	r := deploy.Apply(entry("files/.gitconfig", dst), opts)
	if r.Action != "unchanged" {
		t.Errorf("second apply action: got %q, want %q", r.Action, "unchanged")
	}
}

func TestApply_ForceCopy(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	opts.ForceCopy = true
	dst := homeDst(t, opts, ".gitconfig")

	r := deploy.Apply(entry("files/.gitconfig", dst), opts)
	if r.Action != "copied" {
		t.Errorf("action: got %q, want %q", r.Action, "copied")
	}
	// Should not be a symlink.
	if _, err := os.Readlink(dst); err == nil {
		t.Errorf("dst should not be a symlink")
	}
}

// ── Edge cases ────────────────────────────────────────────────────────────────

func TestApply_MissingSrc(t *testing.T) {
	_, opts := makeRepo(t, nil)
	dst := homeDst(t, opts, "file")

	r := deploy.Apply(entry("files/nonexistent", dst), opts)
	if r.Action != "missing" {
		t.Errorf("action: got %q, want %q", r.Action, "missing")
	}
}

func TestApply_CreatesParentDir(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.config/nvim/init.lua": "-- nvim"})
	dst := homeDst(t, opts, ".config/nvim/init.lua")

	r := deploy.Apply(entry("files/.config/nvim/init.lua", dst), opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if _, err := os.Lstat(dst); err != nil {
		t.Errorf("dst not created: %v", err)
	}
}

func TestApply_DryRun(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	opts.DryRun = true
	dst := homeDst(t, opts, ".gitconfig")

	r := deploy.Apply(entry("files/.gitconfig", dst), opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	// Dry run must not create the file.
	if _, err := os.Lstat(dst); err == nil {
		t.Errorf("dry run must not create dst")
	}
}

func TestApply_SkipsTemplate(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.gitconfig.j2": "template"})
	dst := homeDst(t, opts, ".gitconfig")
	e := entry("files/.gitconfig.j2", dst)
	e.Template = true

	r := deploy.Apply(e, opts)
	if r.Action != "skipped" {
		t.Errorf("action: got %q, want %q", r.Action, "skipped")
	}
}

func TestApply_SkipsSecret(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.ssh/id_rsa.age": "enc"})
	dst := homeDst(t, opts, ".ssh/id_rsa")
	e := entry("files/.ssh/id_rsa.age", dst)
	e.Secret = true

	r := deploy.Apply(e, opts)
	if r.Action != "skipped" {
		t.Errorf("action: got %q, want %q", r.Action, "skipped")
	}
}

func TestApply_LinkEntryOverridesMode(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	opts.DefaultMode = "copy"
	dst := homeDst(t, opts, ".gitconfig")
	e := entry("files/.gitconfig", dst)
	link := true
	e.Link = &link

	r := deploy.Apply(e, opts)
	if r.Action != "linked" {
		t.Errorf("link=true should force symlink: got %q", r.Action)
	}
}

// TestApply_PlatformFilter checks the multi-tag matching rule: an entry with a
// non-empty Only is active when Only intersects opts.Platforms, skipped
// otherwise. On WSL, Platforms() is ["linux","wsl"], so an only=["wsl"] entry
// must apply there but not on a plain linux host.
func TestApply_PlatformFilter(t *testing.T) {
	cases := []struct {
		name      string
		only      []string
		platforms []string
		want      string
	}{
		{"wsl-only on wsl host", []string{"wsl"}, []string{"linux", "wsl"}, "linked"},
		{"wsl-only on plain linux", []string{"wsl"}, []string{"linux"}, "skipped"},
		{"linux-only on wsl host", []string{"linux"}, []string{"linux", "wsl"}, "linked"},
		{"darwin-only on wsl host", []string{"darwin"}, []string{"linux", "wsl"}, "skipped"},
		{"no platforms configured", []string{"linux"}, nil, "skipped"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
			opts.Platforms = tc.platforms
			dst := homeDst(t, opts, ".gitconfig")
			e := entry("files/.gitconfig", dst)
			e.Only = tc.only

			r := deploy.Apply(e, opts)
			if r.Err != nil {
				t.Fatalf("unexpected error: %v", r.Err)
			}
			if r.Action != tc.want {
				t.Errorf("action: got %q, want %q", r.Action, tc.want)
			}
		})
	}
}
