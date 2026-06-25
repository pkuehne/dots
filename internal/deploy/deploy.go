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
	"github.com/pkuehne/dots/internal/parallel"
	"github.com/pkuehne/dots/internal/secrets"
	"github.com/pkuehne/dots/internal/ui"
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
	// Live is set by ApplyAllLive when the entry was shown by a live progress
	// task, so the caller can avoid re-printing it as a residual status line.
	Live bool
}

// Apply deploys a single FileEntry and returns the result, without live
// progress reporting. It is the convenience entry point for callers (and tests)
// that do not drive a UI.
func Apply(entry config.FileEntry, opts Options) Result {
	return ApplyWithTask(entry, opts, ui.DiscardProgress().Task(""))
}

// ApplyWithTask deploys a single FileEntry, reporting its internal stages
// (checking → backing up → linking/copying/decrypting) through task so a live
// progress bar can advance. A no-op, skip, or dry-run prediction reports no
// stages — there is no work to show.
func ApplyWithTask(entry config.FileEntry, opts Options, task ui.Task) Result {
	plan, terminal := prepare(entry, opts)
	if terminal != nil {
		return *terminal
	}
	return execute(entry, plan, opts, task)
}

// ApplyAll deploys all entries sequentially and returns one Result per entry.
func ApplyAll(entries []config.FileEntry, opts Options) []Result {
	results := make([]Result, len(entries))
	for i, e := range entries {
		results[i] = Apply(e, opts)
	}
	return results
}

// ApplyAllLive deploys all entries concurrently (up to jobs at a time), driving
// a live progress task per entry that performs real work through prog. Entries
// that are skipped, missing, or only predicted (dry-run) do no work and open no
// task — the caller surfaces them as plain status lines. Results are returned in
// entry order.
func ApplyAllLive(entries []config.FileEntry, opts Options, prog ui.Progress, jobs int) []Result {
	results := make([]Result, len(entries))
	parallel.Run(entries, jobs, func(i int, e config.FileEntry) {
		plan, terminal := prepare(e, opts)
		if terminal != nil {
			results[i] = *terminal
			return
		}
		task := prog.Task(deployName(e))
		res := execute(e, plan, opts, task)
		res.Live = true
		if res.Err != nil {
			task.Fail(res.Err)
		} else {
			task.Done(res.Action)
		}
		results[i] = res
	})
	return results
}

// deployName is the short label shown on an entry's progress row: its
// configured destination, matching the path printed in the status lines.
func deployName(e config.FileEntry) string {
	return e.Dst
}

// deployPlan is the prepared, validated work for one entry once every cheap,
// non-mutating check has passed and the entry is known to need a live mutation.
type deployPlan struct {
	srcAbs string
	dstAbs string
	secret bool
	link   bool
}

// prepare runs every cheap, non-mutating check for an entry — profile/platform
// filters, mode validation, repo/home path-escape validation, source existence,
// and (for dry-run) action prediction. It returns either a terminal *Result
// (the entry is skipped/missing/errored, or dry-run wants only a prediction) or
// a plan describing the mutation to execute; exactly one is non-nil.
func prepare(entry config.FileEntry, opts Options) (deployPlan, *Result) {
	// Profile filter.
	if entry.Profile != "" && entry.Profile != opts.ActiveProfile {
		return deployPlan{}, &Result{Entry: entry, Action: "skipped"}
	}

	// Platform filter.
	if len(entry.Only) > 0 && !intersects(entry.Only, opts.Platforms) {
		return deployPlan{}, &Result{Entry: entry, Action: "skipped"}
	}

	// Validate the mode string up front so an invalid one is reported (also in
	// dry-run) before any side effect.
	if entry.Mode != "" {
		if _, err := parseFileMode(entry.Mode); err != nil {
			return deployPlan{}, &Result{Entry: entry, Err: err}
		}
	}

	src := filepath.Join(opts.RepoRoot, entry.Src)
	dst := fileutil.Expand(entry.Dst)

	// Validate src is within the repo root.
	repoAbs, err := filepath.Abs(opts.RepoRoot)
	if err != nil {
		return deployPlan{}, &Result{Entry: entry, Err: err}
	}
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return deployPlan{}, &Result{Entry: entry, Err: err}
	}
	if !strings.HasPrefix(srcAbs+string(filepath.Separator), repoAbs+string(filepath.Separator)) {
		return deployPlan{}, &Result{Entry: entry, Err: errs.New(
			fmt.Sprintf("refusing to deploy %q — source escapes repo root", entry.Src),
			fmt.Sprintf("Resolved: %s\nRepo root: %s", srcAbs, repoAbs),
		)}
	}

	// Validate dst is within home.
	home := opts.HomeDir
	if home == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return deployPlan{}, &Result{Entry: entry, Err: err}
		}
	}
	dstParentAbs, err := filepath.Abs(filepath.Dir(dst))
	if err != nil {
		return deployPlan{}, &Result{Entry: entry, Err: err}
	}
	dstAbs := filepath.Join(dstParentAbs, filepath.Base(dst))
	if dstAbs != home && !strings.HasPrefix(dstAbs+string(filepath.Separator), home+string(filepath.Separator)) {
		return deployPlan{}, &Result{Entry: entry, Err: errs.New(
			fmt.Sprintf("refusing to deploy to %q — destination is outside $HOME", entry.Dst),
			fmt.Sprintf("Resolved: %s\nHome: %s", dstAbs, home),
		)}
	}

	if _, err := os.Stat(src); os.IsNotExist(err) {
		return deployPlan{}, &Result{Entry: entry, Action: "missing"}
	}

	// Secrets: decrypt the .age source and write the plaintext to dst (always
	// copy mode, 0600 — never symlink a decrypted secret back into the repo).
	if entry.Secret {
		if opts.DryRun {
			// Dry-run reports intent only; never invokes age.
			return deployPlan{}, &Result{Entry: entry, Action: "decrypt"}
		}
		return deployPlan{srcAbs: srcAbs, dstAbs: dstAbs, secret: true}, nil
	}

	useLink := shouldSymlink(entry, opts)
	if opts.DryRun {
		// Predict the action apply would take so preview and apply agree on a
		// clean system. Mirrors the no-op detection in applySymlink/applyCopy.
		return deployPlan{}, &Result{Entry: entry, Action: predictAction(entry, srcAbs, dstAbs, useLink)}
	}
	return deployPlan{srcAbs: srcAbs, dstAbs: dstAbs, link: useLink}, nil
}

// execute performs the validated mutation in plan, reporting stages through
// task. It is only reached for entries that do real work (prepare returned no
// terminal result), so each call advances and completes a live bar.
func execute(entry config.FileEntry, plan deployPlan, opts Options, task ui.Task) Result {
	// A small step budget so the bar visibly fills; Done forces 100% regardless
	// of the exact count, so the steps need only be approximate.
	task.SetTotal(3)
	task.Stage("checking")
	task.Advance(1)

	if err := fileutil.EnsureParent(plan.dstAbs); err != nil {
		return Result{Entry: entry, Err: err}
	}
	switch {
	case plan.secret:
		return applySecret(entry, plan.srcAbs, plan.dstAbs, opts, task)
	case plan.link:
		return applySymlink(entry, plan.srcAbs, plan.dstAbs, task)
	default:
		return applyCopy(entry, plan.srcAbs, plan.dstAbs, task)
	}
}

// Status returns the current deployment state of an entry without modifying
// anything.
func Status(entry config.FileEntry, opts Options) Result {
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

func applySymlink(entry config.FileEntry, src, dst string, task ui.Task) Result {
	// Already a correct symlink → no-op.
	if target, err := os.Readlink(dst); err == nil && target == src {
		return Result{Entry: entry, Action: "unchanged"}
	}

	// Something else exists at dst → back it up first.
	if _, err := os.Lstat(dst); err == nil {
		task.Stage("backing up")
		if _, err := fileutil.Backup(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
		if err := os.Remove(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
		task.Advance(1)
	}

	task.Stage("linking")
	if err := os.Symlink(src, dst); err != nil {
		return Result{Entry: entry, Err: err}
	}
	task.Advance(1)
	return Result{Entry: entry, Action: "linked"}
}

func applyCopy(entry config.FileEntry, src, dst string, task ui.Task) Result {
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
		task.Stage("backing up")
		if _, err := fileutil.Backup(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
		if err := os.Remove(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
		task.Advance(1)
	}

	task.Stage("copying")
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
	task.Advance(1)
	if err := applyEntryMode(dst, entry.Mode); err != nil {
		return Result{Entry: entry, Err: err}
	}
	return Result{Entry: entry, Action: "copied"}
}

// applySecret decrypts the .age source in-memory and writes the plaintext to
// dst with mode 0600. If dst already holds the same plaintext it is left
// untouched (unchanged); otherwise the existing file is backed up first.
func applySecret(entry config.FileEntry, src, dst string, opts Options, task ui.Task) Result {
	task.Stage("decrypting")
	cfg := config.Config{Secrets: opts.Secrets}
	plaintext, err := secrets.DecryptToMemory(src, cfg)
	if err != nil {
		return Result{Entry: entry, Err: err}
	}
	task.Advance(1)

	if _, err := os.Lstat(dst); err == nil {
		if existing, rerr := os.ReadFile(dst); rerr == nil && bytes.Equal(existing, plaintext) {
			if err := applyEntryMode(dst, entry.Mode); err != nil {
				return Result{Entry: entry, Err: err}
			}
			return Result{Entry: entry, Action: "unchanged"}
		}
		task.Stage("backing up")
		if _, err := fileutil.Backup(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
		if err := os.Remove(dst); err != nil {
			return Result{Entry: entry, Err: err}
		}
	}

	task.Stage("writing")
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
