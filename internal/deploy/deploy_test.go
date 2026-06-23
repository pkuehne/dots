package deploy_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"

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

func TestApply_CopyConvertsSymlink(t *testing.T) {
	root, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	dst := homeDst(t, opts, ".gitconfig")

	// First deploy as a symlink, then switch to copy mode.
	if r := deploy.Apply(entry("files/.gitconfig", dst), opts); r.Action != "linked" {
		t.Fatalf("setup: got %q, want linked", r.Action)
	}
	opts.ForceCopy = true

	r := deploy.Apply(entry("files/.gitconfig", dst), opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if r.Action != "copied" {
		t.Errorf("action: got %q, want %q (symlink must be converted)", r.Action, "copied")
	}
	if _, err := os.Readlink(dst); err == nil {
		t.Error("dst should be a regular file after conversion")
	}
	// The old symlink was backed up before replacement.
	if _, err := os.Lstat(dst + ".dots-bak"); err != nil {
		t.Errorf("backup not created: %v", err)
	}
	_ = root
}

// ── File modes ────────────────────────────────────────────────────────────────

func TestApply_CopyAppliesMode(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.netrc": "machine x"})
	opts.DefaultMode = "copy"
	dst := homeDst(t, opts, ".netrc")
	e := entry("files/.netrc", dst)
	e.Mode = "600"

	r := deploy.Apply(e, opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode: got %v, want 0600", info.Mode().Perm())
	}
}

func TestApply_CopyModeReappliedWhenUnchanged(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.netrc": "machine x"})
	opts.DefaultMode = "copy"
	dst := homeDst(t, opts, ".netrc")
	e := entry("files/.netrc", dst)
	e.Mode = "600"

	deploy.Apply(e, opts)
	if err := os.Chmod(dst, 0o644); err != nil {
		t.Fatal(err)
	}

	r := deploy.Apply(e, opts)
	if r.Action != "unchanged" {
		t.Errorf("action: got %q, want unchanged", r.Action)
	}
	info, _ := os.Stat(dst)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode should be restored on the unchanged path: got %v", info.Mode().Perm())
	}
}

func TestApply_InvalidMode(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.netrc": "machine x"})
	opts.DefaultMode = "copy"
	dst := homeDst(t, opts, ".netrc")
	e := entry("files/.netrc", dst)
	e.Mode = "rw-r--r--"

	r := deploy.Apply(e, opts)
	if r.Err == nil {
		t.Fatal("expected error for non-octal mode string")
	}
	if _, err := os.Lstat(dst); err == nil {
		t.Error("invalid mode must be rejected before any side effect")
	}
}

func TestApply_SymlinkIgnoresMode(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	dst := homeDst(t, opts, ".gitconfig")
	e := entry("files/.gitconfig", dst)
	e.Mode = "600"

	r := deploy.Apply(e, opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if r.Action != "linked" {
		t.Errorf("action: got %q, want linked (mode is ignored for symlinks)", r.Action)
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

// F8: dry-run preview must agree with apply on a clean system. An entry
// already correctly deployed reports "unchanged", not "link"/"copy".
func TestApply_DryRunReportsUnchanged(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	dst := homeDst(t, opts, ".gitconfig")
	e := entry("files/.gitconfig", dst)

	// First a real apply to create the symlink.
	if r := deploy.Apply(e, opts); r.Action != "linked" {
		t.Fatalf("setup apply: got %q, want linked", r.Action)
	}

	opts.DryRun = true
	if r := deploy.Apply(e, opts); r.Action != "unchanged" {
		t.Errorf("dry-run on clean system: got %q, want unchanged", r.Action)
	}

	// And a fresh dst still previews the deploy action.
	e2 := entry("files/.gitconfig", homeDst(t, opts, ".other"))
	if r := deploy.Apply(e2, opts); r.Action != "link" {
		t.Errorf("dry-run on missing dst: got %q, want link", r.Action)
	}
}

// F9: a regular file with identical content where the entry wants a symlink
// is drifted (apply would relink), not "unchanged".
func TestStatus_RegularFileWantedSymlink(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	dst := homeDst(t, opts, ".gitconfig")
	// Place identical content as a real file at dst.
	if err := os.WriteFile(dst, []byte("[user]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := deploy.Status(entry("files/.gitconfig", dst), opts); r.Action != "diff" {
		t.Errorf("status: got %q, want diff", r.Action)
	}
}

// A .j2 file has no special meaning — it is deployed verbatim like any other
// opaque file.
func TestApply_J2DeployedVerbatim(t *testing.T) {
	root, opts := makeRepo(t, map[string]string{"files/.gitconfig.j2": "{{ var }}"})
	dst := homeDst(t, opts, ".gitconfig.j2")

	r := deploy.Apply(entry("files/.gitconfig.j2", dst), opts)
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
	if target != filepath.Join(root, "files/.gitconfig.j2") {
		t.Errorf("symlink target: got %q", target)
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

// ── Secrets ───────────────────────────────────────────────────────────────────

// TestApply_SecretDryRun reports the "decrypt" action without touching disk and
// without invoking age — PATH is emptied to prove age is never run.
func TestApply_SecretDryRun(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.ssh/id_rsa.age": "enc"})
	opts.DryRun = true
	t.Setenv("PATH", t.TempDir())
	dst := homeDst(t, opts, ".ssh/id_rsa")
	e := entry("files/.ssh/id_rsa.age", dst)
	e.Secret = true

	r := deploy.Apply(e, opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if r.Action != "decrypt" {
		t.Errorf("action: got %q, want %q", r.Action, "decrypt")
	}
	if _, err := os.Lstat(dst); err == nil {
		t.Errorf("dry run must not create dst")
	}
}

// TestApply_SecretMissingIdentity surfaces the decryption failure on Result.Err
// rather than silently skipping when no identity is configured.
func TestApply_SecretMissingIdentity(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.ssh/id_rsa.age": "enc"})
	dst := homeDst(t, opts, ".ssh/id_rsa")
	e := entry("files/.ssh/id_rsa.age", dst)
	e.Secret = true

	r := deploy.Apply(e, opts)
	if r.Err == nil {
		t.Fatal("expected error when no identity is configured")
	}
}

// encryptedSecret generates an age keypair, encrypts plaintext to <root>/<rel>,
// and returns the identity file path. Uses the linked-in age library, so it
// needs no external binaries.
func encryptedSecret(t *testing.T, root, rel, plaintext string) string {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity: %v", err)
	}
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key.txt")
	contents := "# public key: " + id.Recipient().String() + "\n" + id.String() + "\n"
	if err := os.WriteFile(keyFile, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	ageFile := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(ageFile), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, id.Recipient())
	if err != nil {
		t.Fatalf("age.Encrypt: %v", err)
	}
	if _, err := w.Write([]byte(plaintext)); err != nil {
		t.Fatalf("write plaintext: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close encryptor: %v", err)
	}
	if err := os.WriteFile(ageFile, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return keyFile
}

// TestApply_SecretDecrypts performs a real decrypt round-trip: the plaintext is
// written to dst (0600, not a symlink), and a second apply is idempotent.
func TestApply_SecretDecrypts(t *testing.T) {
	root, opts := makeRepo(t, nil)
	identity := encryptedSecret(t, root, "files/.ssh/id_rsa.age", "super-secret")
	opts.Secrets.Identity = identity
	dst := homeDst(t, opts, ".ssh/id_rsa")
	e := entry("files/.ssh/id_rsa.age", dst)
	e.Secret = true

	r := deploy.Apply(e, opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	if r.Action != "decrypted" {
		t.Errorf("action: got %q, want %q", r.Action, "decrypted")
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("dst not written: %v", err)
	}
	if string(got) != "super-secret" {
		t.Errorf("content: got %q, want %q", got, "super-secret")
	}
	info, err := os.Lstat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("decrypted secret must not be a symlink")
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode: got %v, want 0600", info.Mode().Perm())
	}

	// Idempotent: re-applying the same secret leaves it unchanged.
	if r2 := deploy.Apply(e, opts); r2.Action != "unchanged" {
		t.Errorf("second apply: got %q, want unchanged", r2.Action)
	}
}

// TestApply_SecretCustomMode lets a [[file]] mode override the 0600 default.
func TestApply_SecretCustomMode(t *testing.T) {
	root, opts := makeRepo(t, nil)
	identity := encryptedSecret(t, root, "files/.config/token.age", "tok")
	opts.Secrets.Identity = identity
	dst := homeDst(t, opts, ".config/token")
	e := entry("files/.config/token.age", dst)
	e.Secret = true
	e.Mode = "640"

	r := deploy.Apply(e, opts)
	if r.Err != nil {
		t.Fatalf("unexpected error: %v", r.Err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode: got %v, want 0640", info.Mode().Perm())
	}
}

// ── Status ────────────────────────────────────────────────────────────────────

// TestStatus_SymlinkWantedCopy: a correct symlink is "diff", not "linked", when
// the entry asks for copy mode — apply would convert it.
func TestStatus_SymlinkWantedCopy(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/.gitconfig": "[user]"})
	dst := homeDst(t, opts, ".gitconfig")
	if r := deploy.Apply(entry("files/.gitconfig", dst), opts); r.Action != "linked" {
		t.Fatalf("setup: got %q, want linked", r.Action)
	}

	if r := deploy.Status(entry("files/.gitconfig", dst), opts); r.Action != "linked" {
		t.Errorf("symlink mode status: got %q, want linked", r.Action)
	}

	opts.DefaultMode = "copy"
	if r := deploy.Status(entry("files/.gitconfig", dst), opts); r.Action != "diff" {
		t.Errorf("copy mode status: got %q, want diff", r.Action)
	}
}
