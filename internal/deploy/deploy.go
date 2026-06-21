// Package deploy deploys FileEntry values to their destinations by symlinking
// or copying, with idempotency and backup on overwrite.
package deploy

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/fileutil"
	"github.com/pkuehne/dots/internal/secrets"
)

// Options controls deploy behaviour.
type Options struct {
	DryRun        bool
	ForceCopy     bool // treat every entry as copy regardless of config
	RepoRoot      string
	DefaultMode   string // "symlink" (default) or "copy"
	ActiveProfile string
	Platforms     []string             // active platform tags (platform.Platforms())
	Secrets       config.SecretsConfig // age identity/recipient for .age decryption
	HomeDir       string               // override os.UserHomeDir(); used in tests
}

// Result describes what happened when a single file was deployed.
type Result struct {
	Entry  config.FileEntry
	Action string // "linked", "copied", "unchanged", "skipped", "missing"
	Err    error
}

// Apply deploys a single FileEntry and returns the result.
func Apply(entry config.FileEntry, opts Options) Result {
	// Templates (.j2) are not supported in the Go version — there is no Jinja2
	// renderer. Skip them, but make the reason visible.
	if entry.Template {
		return Result{Entry: entry, Action: "skipped (template — not supported)"}
	}

	// Profile filter.
	if entry.Profile != "" && entry.Profile != opts.ActiveProfile {
		return Result{Entry: entry, Action: "skipped"}
	}

	// Platform filter.
	if len(entry.Only) > 0 && !intersects(entry.Only, opts.Platforms) {
		return Result{Entry: entry, Action: "skipped"}
	}

	// Validate the mode string up front so an invalid one is reported (also in
	// dry-run) before any side effect.
	if entry.Mode != "" {
		if _, err := parseFileMode(entry.Mode); err != nil {
			return Result{Entry: entry, Err: err}
		}
	}

	src := filepath.Join(opts.RepoRoot, entry.Src)
	dst := fileutil.Expand(entry.Dst)

	// Validate src is within the repo root.
	repoAbs, err := filepath.Abs(opts.RepoRoot)
	if err != nil {
		return Result{Entry: entry, Err: err}
	}
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return Result{Entry: entry, Err: err}
	}
	if !strings.HasPrefix(srcAbs+string(filepath.Separator), repoAbs+string(filepath.Separator)) {
		return Result{Entry: entry, Err: errs.New(
			fmt.Sprintf("refusing to deploy %q — source escapes repo root", entry.Src),
			fmt.Sprintf("Resolved: %s\nRepo root: %s", srcAbs, repoAbs),
		)}
	}

	// Validate dst is within home.
	home := opts.HomeDir
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return Result{Entry: entry, Err: err}
		}
	}
	dstParentAbs, err := filepath.Abs(filepath.Dir(dst))
	if err != nil {
		return Result{Entry: entry, Err: err}
	}
	dstAbs := filepath.Join(dstParentAbs, filepath.Base(dst))
	if dstAbs != home && !strings.HasPrefix(dstAbs+string(filepath.Separator), home+string(filepath.Separator)) {
		return Result{Entry: entry, Err: errs.New(
			fmt.Sprintf("refusing to deploy to %q — destination is outside $HOME", entry.Dst),
			fmt.Sprintf("Resolved: %s\nHome: %s", dstAbs, home),
		)}
	}

	if _, err := os.Stat(src); os.IsNotExist(err) {
		return Result{Entry: entry, Action: "missing"}
	}

	// Secrets: decrypt the .age source and write the plaintext to dst (always
	// copy mode, 0600 — never symlink a decrypted secret back into the repo).
	if entry.Secret {
		if opts.DryRun {
			// Dry-run reports intent only; never invokes age.
			return Result{Entry: entry, Action: "decrypt"}
		}
		if err := fileutil.EnsureParent(dstAbs); err != nil {
			return Result{Entry: entry, Err: err}
		}
		return applySecret(entry, srcAbs, dstAbs, opts)
	}

	useLink := shouldSymlink(entry, opts)

	if opts.DryRun {
		// Predict the action apply would take so preview and apply agree on a
		// clean system. Mirrors the no-op detection in applySymlink/applyCopy.
		return Result{Entry: entry, Action: predictAction(entry, srcAbs, dstAbs, useLink)}
	}

	if err := fileutil.EnsureParent(dstAbs); err != nil {
		return Result{Entry: entry, Err: err}
	}

	if useLink {
		return applySymlink(entry, srcAbs, dstAbs)
	}
	return applyCopy(entry, srcAbs, dstAbs)
}

// ApplyAll deploys all entries and returns one Result per entry.
func ApplyAll(entries []config.FileEntry, opts Options) []Result {
	results := make([]Result, len(entries))
	for i, e := range entries {
		results[i] = Apply(e, opts)
	}
	return results
}

// Status returns the current deployment state of an entry without modifying
// anything.
func Status(entry config.FileEntry, opts Options) Result {
	if entry.Template {
		return Result{Entry: entry, Action: "skipped (template — not supported)"}
	}
	if entry.Secret {
		// Reporting real status would require decrypting (and thus age); report
		// it as a secret (apply performs the decryption) so it stays visible
		// rather than being hidden with the inactive "skipped" entries.
		return Result{Entry: entry, Action: "secret"}
	}
	if entry.Profile != "" && entry.Profile != opts.ActiveProfile {
		return Result{Entry: entry, Action: "skipped"}
	}
	if len(entry.Only) > 0 && !intersects(entry.Only, opts.Platforms) {
		return Result{Entry: entry, Action: "skipped"}
	}

	src := filepath.Join(opts.RepoRoot, entry.Src)
	dst := fileutil.Expand(entry.Dst)

	if _, err := os.Lstat(dst); os.IsNotExist(err) {
		return Result{Entry: entry, Action: "missing"}
	}

	// Symlink: check if it points to src — and that the entry actually wants a
	// symlink; a correct link still differs when the entry asks for a copy.
	if target, err := os.Readlink(dst); err == nil {
		srcAbs, _ := filepath.Abs(src)
		if target == srcAbs && shouldSymlink(entry, opts) {
			return Result{Entry: entry, Action: "linked"}
		}
		return Result{Entry: entry, Action: "diff"}
	}

	// Regular file where the entry wants a symlink: apply would back it up and
	// relink, so it is drifted even if the content currently matches.
	if shouldSymlink(entry, opts) {
		return Result{Entry: entry, Action: "diff"}
	}

	// Regular file in copy mode: compare hashes.
	srcHash, err1 := fileutil.SHA256File(src)
	dstHash, err2 := fileutil.SHA256File(dst)
	if err1 != nil || err2 != nil {
		return Result{Entry: entry, Action: "missing"}
	}
	if srcHash == dstHash {
		return Result{Entry: entry, Action: "unchanged"}
	}
	return Result{Entry: entry, Action: "diff"}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// predictAction reports the action Apply would take for entry without touching
// the filesystem, so --dry-run preview matches a real apply on a clean system.
func predictAction(entry config.FileEntry, src, dst string, useLink bool) string {
	if useLink {
		// A correct symlink already in place is a no-op.
		if target, err := os.Readlink(dst); err == nil && target == src {
			return "unchanged"
		}
		return "link"
	}
	// Copy mode: an existing real file with identical content is a no-op. An
	// existing symlink is always replaced (applyCopy follows the same rule).
	if info, err := os.Lstat(dst); err == nil && info.Mode()&os.ModeSymlink == 0 {
		srcHash, e1 := fileutil.SHA256File(src)
		dstHash, e2 := fileutil.SHA256File(dst)
		if e1 == nil && e2 == nil && srcHash == dstHash {
			return "unchanged"
		}
	}
	return "copy"
}

func applySymlink(entry config.FileEntry, src, dst string) Result {
	// Already a correct symlink → no-op.
	if target, err := os.Readlink(dst); err == nil && target == src {
		return Result{Entry: entry, Action: "unchanged"}
	}

	// Something else exists at dst → back it up first.
	if _, err := os.Lstat(dst); err == nil {
		if _, err := fileutil.Backup(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
		if err := os.Remove(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
	}

	if err := os.Symlink(src, dst); err != nil {
		return Result{Entry: entry, Err: err}
	}
	return Result{Entry: entry, Action: "linked"}
}

func applyCopy(entry config.FileEntry, src, dst string) Result {
	if info, err := os.Lstat(dst); err == nil {
		// An existing symlink is always replaced with a real copy — even one
		// pointing at identical content (hashing follows the link), otherwise
		// copy mode could never convert a previously symlinked deployment.
		if info.Mode()&os.ModeSymlink == 0 {
			// Same content → no-op (just apply mode if set).
			srcHash, e1 := fileutil.SHA256File(src)
			dstHash, e2 := fileutil.SHA256File(dst)
			if e1 == nil && e2 == nil && srcHash == dstHash {
				if err := applyEntryMode(dst, entry.Mode); err != nil {
					return Result{Entry: entry, Err: err}
				}
				return Result{Entry: entry, Action: "unchanged"}
			}
		}
		if _, err := fileutil.Backup(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
		if err := os.Remove(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return Result{Entry: entry, Err: err}
	}
	in, err := os.Open(src)
	if err != nil {
		return Result{Entry: entry, Err: err}
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return Result{Entry: entry, Err: err}
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return Result{Entry: entry, Err: err}
	}
	if err := applyEntryMode(dst, entry.Mode); err != nil {
		return Result{Entry: entry, Err: err}
	}
	return Result{Entry: entry, Action: "copied"}
}

// applySecret decrypts the .age source in-memory and writes the plaintext to
// dst with mode 0600. If dst already holds the same plaintext it is left
// untouched (unchanged); otherwise the existing file is backed up first.
func applySecret(entry config.FileEntry, src, dst string, opts Options) Result {
	cfg := config.Config{Secrets: opts.Secrets}
	plaintext, err := secrets.DecryptToMemory(src, cfg)
	if err != nil {
		return Result{Entry: entry, Err: err}
	}

	if _, err := os.Lstat(dst); err == nil {
		if existing, rerr := os.ReadFile(dst); rerr == nil && bytes.Equal(existing, plaintext) {
			if err := applyEntryMode(dst, entry.Mode); err != nil {
				return Result{Entry: entry, Err: err}
			}
			return Result{Entry: entry, Action: "unchanged"}
		}
		if _, err := fileutil.Backup(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
		if err := os.Remove(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
	}

	if err := os.WriteFile(dst, plaintext, 0o600); err != nil {
		return Result{Entry: entry, Err: err}
	}
	// entry.Mode overrides the 0600 default; chmod explicitly because
	// os.WriteFile's permission argument is filtered by the umask.
	if err := applyEntryMode(dst, entry.Mode); err != nil {
		return Result{Entry: entry, Err: err}
	}
	return Result{Entry: entry, Action: "decrypted"}
}

// parseFileMode parses the octal permission string of a [[file]] entry
// (e.g. "600" or "0644").
func parseFileMode(s string) (os.FileMode, error) {
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, errs.New(
			fmt.Sprintf("invalid file mode %q", s),
			`Use an octal permission string in dots.toml, e.g. mode = "600".`,
		)
	}
	return os.FileMode(v), nil
}

// applyEntryMode chmods dst to the entry's mode string. An empty mode is a
// no-op. Symlink entries never reach this — a mode on a symlink is meaningless
// and is ignored, matching the Python behavior.
func applyEntryMode(dst, mode string) error {
	if mode == "" {
		return nil
	}
	m, err := parseFileMode(mode)
	if err != nil {
		return err
	}
	return os.Chmod(dst, m)
}

func shouldSymlink(entry config.FileEntry, opts Options) bool {
	if entry.Link != nil {
		return *entry.Link
	}
	if opts.ForceCopy {
		return false
	}
	return opts.DefaultMode != "copy"
}

// intersects reports whether any element of only is present in platforms.
// An entry with a non-empty Only is active only on a matching platform tag.
func intersects(only, platforms []string) bool {
	for _, o := range only {
		for _, p := range platforms {
			if o == p {
				return true
			}
		}
	}
	return false
}
