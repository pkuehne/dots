// Package deploy deploys FileEntry values to their destinations by symlinking
// or copying, with idempotency and backup on overwrite.
package deploy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/fileutil"
)

// Options controls deploy behaviour.
type Options struct {
	DryRun        bool
	ForceCopy     bool // treat every entry as copy regardless of config
	RepoRoot      string
	DefaultMode   string // "symlink" (default) or "copy"
	ActiveProfile string
	Platforms     []string // active platform tags (platform.Platforms())
	HomeDir       string   // override os.UserHomeDir(); used in tests
}

// Result describes what happened when a single file was deployed.
type Result struct {
	Entry  config.FileEntry
	Action string // "linked", "copied", "unchanged", "skipped", "missing"
	Err    error
}

// Apply deploys a single FileEntry and returns the result.
func Apply(entry config.FileEntry, opts Options) Result {
	// Skip templates and secrets — not yet implemented.
	if entry.Template || entry.Secret {
		return Result{Entry: entry, Action: "skipped"}
	}

	// Profile filter.
	if entry.Profile != "" && entry.Profile != opts.ActiveProfile {
		return Result{Entry: entry, Action: "skipped"}
	}

	// Platform filter.
	if len(entry.Only) > 0 && !intersects(entry.Only, opts.Platforms) {
		return Result{Entry: entry, Action: "skipped"}
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

	useLink := shouldSymlink(entry, opts)

	if opts.DryRun {
		action := "copy"
		if useLink {
			action = "link"
		}
		return Result{Entry: entry, Action: action}
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
	if entry.Template || entry.Secret {
		return Result{Entry: entry, Action: "skipped"}
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

	// Symlink: check if it points to src.
	if target, err := os.Readlink(dst); err == nil {
		srcAbs, _ := filepath.Abs(src)
		if target == srcAbs {
			return Result{Entry: entry, Action: "linked"}
		}
		return Result{Entry: entry, Action: "diff"}
	}

	// Regular file: compare hashes.
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
	// Same content → no-op (just apply mode if set).
	if _, err := os.Lstat(dst); err == nil {
		srcHash, e1 := fileutil.SHA256File(src)
		dstHash, e2 := fileutil.SHA256File(dst)
		if e1 == nil && e2 == nil && srcHash == dstHash {
			return Result{Entry: entry, Action: "unchanged"}
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
	return Result{Entry: entry, Action: "copied"}
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
