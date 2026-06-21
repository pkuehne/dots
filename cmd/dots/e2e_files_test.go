//go:build e2e

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pkuehne/dots/internal/platform"
)

// TestE2E_ApplyDeploysSymlink is the headline path: a managed dotfile is
// symlinked into HOME pointing back at the repo source.
func TestE2E_ApplyDeploysSymlink(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)

	src := filepath.Join(repo, "files", ".testrc")
	writeFile(t, src, "# managed by dots\n")

	out := mustDots(t, home, "--repo", repo, "apply")
	assertContains(t, "apply summary", out, "linked")

	assertSymlinkTo(t, filepath.Join(home, ".testrc"), src)
}

// TestE2E_ApplyNestedDirs confirms files in nested directories deploy to the
// matching nested path under HOME, creating intermediate directories.
func TestE2E_ApplyNestedDirs(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)

	src := filepath.Join(repo, "files", ".config", "nvim", "init.lua")
	writeFile(t, src, "-- nvim config\n")

	mustDots(t, home, "--repo", repo, "apply")
	assertSymlinkTo(t, filepath.Join(home, ".config", "nvim", "init.lua"), src)
}

// TestE2E_ApplyIdempotent asserts invariant 3: a second apply changes nothing
// and still succeeds (same symlink, no churn).
func TestE2E_ApplyIdempotent(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)
	writeFile(t, filepath.Join(repo, "files", ".testrc"), "# managed\n")

	mustDots(t, home, "--repo", repo, "apply")
	dst := filepath.Join(home, ".testrc")
	first, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("readlink after first apply: %v", err)
	}

	out := mustDots(t, home, "--repo", repo, "apply")
	second, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("readlink after second apply: %v", err)
	}
	if first != second {
		t.Errorf("symlink changed between applies: %q -> %q", first, second)
	}
	// Nothing was re-linked the second time round.
	assertContains(t, "second apply", out, "1 unchanged")
}

// TestE2E_DryRunNoSideEffects asserts invariant 5: --dry-run writes nothing.
func TestE2E_DryRunNoSideEffects(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)
	writeFile(t, filepath.Join(repo, "files", ".testrc"), "# managed\n")

	out := mustDots(t, home, "--repo", repo, "apply", "--dry-run")
	assertContains(t, "dry-run mentions the file", out, ".testrc")
	assertNotExists(t, filepath.Join(home, ".testrc"))

	// `preview` is the documented alias for `apply --dry-run`.
	out = mustDots(t, home, "--repo", repo, "preview")
	assertContains(t, "preview mentions the file", out, ".testrc")
	assertNotExists(t, filepath.Join(home, ".testrc"))
}

// TestE2E_BackupOnConflict asserts invariant 8: an existing real file is backed
// up to <name>.dots-bak before being replaced with the managed symlink.
func TestE2E_BackupOnConflict(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)
	writeFile(t, filepath.Join(repo, "files", ".gitconfig"), "[user]\n")

	// Pre-existing, unmanaged file in HOME.
	dst := filepath.Join(home, ".gitconfig")
	writeFile(t, dst, "old content\n")

	mustDots(t, home, "--repo", repo, "apply")

	assertSymlink(t, dst)
	bak := dst + ".dots-bak"
	if got := readFile(t, bak); got != "old content\n" {
		t.Errorf("backup content = %q, want %q", got, "old content\n")
	}
}

// TestE2E_CopyModeAndPerms covers copy mode (link = false) and an explicit
// octal mode on a [[file]] entry — the deployed file is a real file, not a
// symlink, with the requested permissions.
func TestE2E_CopyModeAndPerms(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeFile(t, filepath.Join(repo, "files", ".netrc"), "machine example.com\n")
	writeToml(t, repo, `[meta]
version = 1

[[file]]
src = "files/.netrc"
dst = "~/.netrc"
mode = "600"
link = false
`)

	mustDots(t, home, "--repo", repo, "apply")

	dst := filepath.Join(home, ".netrc")
	assertRegularFile(t, dst)
	assertMode(t, dst, 0o600)
}

// TestE2E_ForceCopyFlag covers `apply --copy`: a normally-symlinked file is
// deployed as a real copy instead.
func TestE2E_ForceCopyFlag(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)
	writeFile(t, filepath.Join(repo, "files", ".marker"), "force copied\n")

	mustDots(t, home, "--repo", repo, "apply", "--copy")

	dst := filepath.Join(home, ".marker")
	assertRegularFile(t, dst)
	if got := readFile(t, dst); got != "force copied\n" {
		t.Errorf("content = %q, want %q", got, "force copied\n")
	}
}

// TestE2E_PlatformFiltering covers files.d/<tag>/ scoping and explicit only=
// guards: the active platform's tree deploys, an inactive platform's does not.
func TestE2E_PlatformFiltering(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)

	active := platform.Detect()
	inactive := "darwin"
	if active == "darwin" {
		inactive = "linux"
	}

	writeFile(t, filepath.Join(repo, "files", ".shared"), "shared\n")
	writeFile(t, filepath.Join(repo, "files.d", active, ".active-file"), "active\n")
	writeFile(t, filepath.Join(repo, "files.d", inactive, ".inactive-file"), "inactive\n")
	writeToml(t, repo, `[meta]
version = 1

[[file]]
src = "files/.guarded"
dst = "~/.guarded"
only = ["`+inactive+`"]
`)
	writeFile(t, filepath.Join(repo, "files", ".guarded"), "guarded\n")

	mustDots(t, home, "--repo", repo, "apply")

	assertSymlink(t, filepath.Join(home, ".shared"))
	assertSymlink(t, filepath.Join(home, ".active-file"))
	assertNotExists(t, filepath.Join(home, ".inactive-file"))
	assertNotExists(t, filepath.Join(home, ".guarded"))
}

// TestE2E_ProfileFiltering covers [[file]] profile scoping: an entry tied to a
// profile only deploys when that profile is active.
func TestE2E_ProfileFiltering(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	writeFile(t, filepath.Join(repo, "files", ".work-only"), "work\n")
	writeToml(t, repo, `[meta]
version = 1

[[file]]
src = "files/.work-only"
dst = "~/.work-only"
profile = "work"
`)

	// Without the profile: skipped.
	mustDots(t, home, "--repo", repo, "apply")
	assertNotExists(t, filepath.Join(home, ".work-only"))

	// With the profile: deployed.
	mustDots(t, home, "--repo", repo, "--profile", "work", "apply")
	assertSymlink(t, filepath.Join(home, ".work-only"))
}

// TestE2E_ApplySingleFile covers the positional `apply <name>` form: only the
// named file is deployed, and an unknown name fails loudly (a likely typo).
func TestE2E_ApplySingleFile(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)
	writeFile(t, filepath.Join(repo, "files", ".one"), "one\n")
	writeFile(t, filepath.Join(repo, "files", ".two"), "two\n")

	mustDots(t, home, "--repo", repo, "apply", ".one")
	assertSymlink(t, filepath.Join(home, ".one"))
	assertNotExists(t, filepath.Join(home, ".two"))

	out, err := dots(t, home, "--repo", repo, "apply", ".nonexistent")
	if err == nil {
		t.Fatalf("apply of unknown file should fail; output:\n%s", out)
	}
	assertContains(t, "unknown file error", out, "no managed file matches")
}

// TestE2E_StatusListDiff covers the read-only inspection commands against a
// freshly applied repo.
func TestE2E_StatusListDiff(t *testing.T) {
	home := t.TempDir()
	repo := initRepo(t, home)
	writeFile(t, filepath.Join(repo, "files", ".testrc"), "# managed\n")
	mustDots(t, home, "--repo", repo, "apply")

	// status: shows the file as linked.
	out := mustDots(t, home, "--repo", repo, "status")
	assertContains(t, "status header", out, "Files:")
	assertContains(t, "status linked", out, "linked")
	assertContains(t, "status file", out, ".testrc")

	// list: shows the managed file.
	out = mustDots(t, home, "--repo", repo, "list")
	assertContains(t, "list file", out, ".testrc")

	// diff: a clean deployment has no diffs.
	out = mustDots(t, home, "--repo", repo, "diff")
	assertContains(t, "no diffs", out, "No diffs found")
}

// TestE2E_ZeroConfig covers mirror mode: a repo with a files/ directory and no
// dots.toml still deploys everything under files/.
func TestE2E_ZeroConfig(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "files", ".bash_aliases"), "alias ll='ls -la'\n")
	writeFile(t, filepath.Join(repo, "files", ".config", "vimrc"), "colorscheme desert\n")

	// No dots.toml written — FindRepoRoot accepts a bare files/ directory.
	mustDots(t, home, "--repo", repo, "apply")
	assertSymlink(t, filepath.Join(home, ".bash_aliases"))
	assertSymlink(t, filepath.Join(home, ".config", "vimrc"))
}

// TestE2E_SecretStatusAndDecrypt covers the secret (.age) path without
// requiring age for the parts that do not need it, and skips the round-trip
// when age is unavailable.
func TestE2E_SecretStatusAndDecrypt(t *testing.T) {
	home := t.TempDir()
	repo := scaffoldRepo(t)
	// A discovered .age file is flagged secret; status reports it without
	// decrypting (so no age binary is required for this assertion).
	writeFile(t, filepath.Join(repo, "files", ".secret-token.age"), "not real ciphertext\n")
	writeToml(t, repo, "[meta]\nversion = 1\n")

	out := mustDots(t, home, "--repo", repo, "status")
	assertContains(t, "status marks secret", out, "secret")

	// `decrypt` rejects a non-.age path before ever invoking age.
	plain := filepath.Join(home, "plain.txt")
	writeFile(t, plain, "hello\n")
	out, err := dots(t, home, "--repo", repo, "decrypt", plain)
	if err == nil {
		t.Fatalf("decrypt of non-.age file should fail; output:\n%s", out)
	}
	assertContains(t, "suffix error", out, ".age")

	if _, err := exec.LookPath("age"); err != nil {
		t.Skip("age not installed — skipping encrypt/decrypt round-trip")
	}
	secretRoundTrip(t, home, repo)
}

// secretRoundTrip exercises a real age keypair: generate a key, encrypt a file,
// deploy it via a secret [[file]] entry, and confirm the plaintext lands at the
// destination with 0600 permissions.
func secretRoundTrip(t *testing.T, home, repo string) {
	t.Helper()
	if _, err := exec.LookPath("age-keygen"); err != nil {
		t.Skip("age-keygen not installed — skipping round-trip")
	}

	keyPath := filepath.Join(home, ".config", "dots", "key.txt")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	keygen := exec.Command("age-keygen", "-o", keyPath)
	if out, err := keygen.CombinedOutput(); err != nil {
		t.Fatalf("age-keygen: %v\n%s", err, out)
	}
	recipient := publicKeyFromIdentity(t, keyPath)

	// Encrypt a plaintext secret into the repo as files/.token.age.
	plainSrc := filepath.Join(t.TempDir(), "token")
	writeFile(t, plainSrc, "s3cr3t\n")
	encOut := filepath.Join(repo, "files", ".token.age")
	enc := exec.Command("age", "--encrypt", "-r", recipient, "-o", encOut, plainSrc)
	if out, err := enc.CombinedOutput(); err != nil {
		t.Fatalf("age encrypt: %v\n%s", err, out)
	}

	writeToml(t, repo, `[meta]
version = 1

[secrets]
identity = "~/.config/dots/key.txt"
recipient = "`+recipient+`"
`)

	mustDots(t, home, "--repo", repo, "apply")

	dst := filepath.Join(home, ".token")
	assertRegularFile(t, dst)
	if got := readFile(t, dst); got != "s3cr3t\n" {
		t.Errorf("decrypted content = %q, want %q", got, "s3cr3t\n")
	}
	assertMode(t, dst, 0o600)
}

// publicKeyFromIdentity extracts the age recipient (public key) from a key file
// produced by age-keygen.
func publicKeyFromIdentity(t *testing.T, keyPath string) string {
	t.Helper()
	out, err := exec.Command("age-keygen", "-y", keyPath).Output()
	if err != nil {
		t.Fatalf("age-keygen -y: %v", err)
	}
	return string(out[:len(out)-1]) // strip trailing newline
}
