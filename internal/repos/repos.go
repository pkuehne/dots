// Package repos clones and updates [[repo]] entries.
package repos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/fileutil"
	"github.com/pkuehne/dots/internal/platform"
	"github.com/pkuehne/dots/internal/ui"
)

// RepoState is the current state of a managed repo clone.
type RepoState struct {
	Entry    config.RepoEntry
	Exists   bool
	Dirty    bool
	Behind   int    // commits behind remote
	Current  string // current ref (branch or SHA)
	Ref      string // configured target ref ("" when tracking the default branch)
	OnTarget bool   // HEAD is at the configured ref (only meaningful when Ref != "")
}

// isLatestRef reports whether a configured ref means "track the default
// branch": empty or the literal "latest" (case-insensitive).
func isLatestRef(ref string) bool {
	return ref == "" || strings.EqualFold(ref, "latest")
}

// CloneResult is the outcome of processing one [[repo]] during a clone pass.
// Action is "cloned" when a clone happened (or would happen in dry-run) and
// "present" when the repo was already on disk. Printing is left to the caller
// so apply can render coloured, uniform status lines.
type CloneResult struct {
	Entry  config.RepoEntry
	Action string
}

// Clone clones any repos that are not yet present on disk and returns a result
// per active repo. If names is non-empty, only those repos are considered.
func Clone(cfg config.Config, names []string, dryRun bool) ([]CloneResult, error) {
	var results []CloneResult
	for _, r := range active(cfg, names) {
		status, err := cloneOne(r, dryRun)
		if err != nil {
			return results, err
		}
		action := "present"
		if status == "ok" {
			action = "cloned"
		}
		results = append(results, CloneResult{Entry: r, Action: action})
	}
	return results, nil
}

// UpdateResult is the outcome of processing one [[repo]] during an update pass.
// Action is one of "updated", "would-update" (dry-run), "skipped-missing" (not
// cloned), or "skipped-dirty" (uncommitted local changes). Printing is left to
// the caller so the command renders uniform status lines.
type UpdateResult struct {
	Entry  config.RepoEntry
	Action string
	Err    error // set when Action is "failed"
}

// DefaultJobs is the number of repos updated concurrently when the caller does
// not specify. Updates are network-bound (git fetch/pull), so a few workers
// overlap the latency.
const DefaultJobs = 4

// Update fetches and pulls repos that already exist, processing up to jobs at a
// time and reporting live progress through prog. Results are returned in config
// order; the caller prints them. The returned error is the first failure
// encountered (other repos still run).
func Update(cfg config.Config, names []string, dryRun bool, prog ui.Progress, jobs int) ([]UpdateResult, error) {
	repos := active(cfg, names)
	results := make([]UpdateResult, len(repos))
	if jobs < 1 {
		jobs = 1
	}

	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	recordErr := func(err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}

	for i, r := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r config.RepoEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = updateRepo(r, dryRun, prog, recordErr)
		}(i, r)
	}
	wg.Wait()
	return results, firstErr
}

// updateRepo updates one repo, driving a progress task through its git stages.
// A skip (not cloned, or dirty) is reported without a task since there is no
// work to show.
func updateRepo(r config.RepoEntry, dryRun bool, prog ui.Progress, recordErr func(error)) UpdateResult {
	// Pre-check the cheap, non-mutating conditions before opening a live row so
	// skipped repos do not flash a task.
	dst := fileutil.Expand(r.Dst)
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return UpdateResult{Entry: r, Action: "skipped-missing"}
	}
	// A shallow update does `git reset --hard`, which would silently discard
	// local modifications. Refuse to update a dirty repo so user work survives.
	// (A normal `git pull` aborts on conflict on its own, so only shallow needs
	// the guard — but checking unconditionally keeps the behaviour uniform.)
	if out, err := gitOutput([]string{"status", "--porcelain"}, dst); err == nil && strings.TrimSpace(out) != "" {
		return UpdateResult{Entry: r, Action: "skipped-dirty"}
	}
	if dryRun {
		return UpdateResult{Entry: r, Action: "would-update"}
	}

	task := prog.Task(r.Name)
	if err := updateOneWithTask(r, dst, task); err != nil {
		task.Fail(err)
		recordErr(err)
		return UpdateResult{Entry: r, Action: "failed", Err: err}
	}
	task.Done("updated")
	return UpdateResult{Entry: r, Action: "updated"}
}

// updateOneWithTask runs the mutating part of an update (the dirty/missing
// guards already passed), reporting git stages through task.
func updateOneWithTask(r config.RepoEntry, dst string, task ui.Task) error {
	task.Stage("fetching")
	if err := syncRef(r, dst); err != nil {
		return err
	}
	if r.OnUpdate != "" {
		task.Stage("running hook")
		if err := shellRun(r.OnUpdate, dst); err != nil {
			return err
		}
	}
	return nil
}

// Status returns the state of all active repo clones.
func Status(cfg config.Config) ([]RepoState, error) {
	repos := active(cfg, nil)
	states := make([]RepoState, 0, len(repos))
	for _, r := range repos {
		s, err := repoState(r)
		if err != nil {
			return nil, err
		}
		states = append(states, s)
	}
	return states, nil
}

// active returns the repos from cfg matching names that are in scope for the
// current platform tags and active profile.
func active(cfg config.Config, names []string) []config.RepoEntry {
	return Filter(cfg.Repos, names, platform.Platforms(), cfg.ActiveProfile)
}

// Filter returns the subset of repos matching names that are active on the
// given platform tags and profile. A repo with a non-empty Only is active
// only when Only intersects platforms; a repo with a non-empty Profile is
// active only when it equals profile. If names is empty, all active repos
// are returned.
func Filter(repos []config.RepoEntry, names []string, platforms []string, profile string) []config.RepoEntry {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	var out []config.RepoEntry
	for _, r := range repos {
		if len(r.Only) > 0 && !intersects(r.Only, platforms) {
			continue
		}
		if r.Profile != "" && r.Profile != profile {
			continue
		}
		if len(names) > 0 && !set[r.Name] {
			continue
		}
		out = append(out, r)
	}
	return out
}

// intersects reports whether any element of only is present in platforms.
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

// cloneOne clones a single repo. Returns "ok", "already", or an error.
func cloneOne(r config.RepoEntry, dryRun bool) (string, error) {
	dst := fileutil.Expand(r.Dst)

	if info, err := os.Stat(dst); err == nil && info.IsDir() {
		if _, gerr := os.Stat(filepath.Join(dst, ".git")); os.IsNotExist(gerr) {
			return "", &errs.DotsError{
				Msg: fmt.Sprintf("Cannot clone %s to %s", r.Name, dst),
				Hint: "Reason: Directory exists but is not a git repository\n\n" +
					"Hint: If you want dots to manage this directory, remove it first:\n" +
					"  rm -rf " + dst + "\n" +
					"Then re-run: dots repos clone " + r.Name + "\n\n" +
					"If you want to keep the existing installation, remove the [[repo]] entry\n" +
					"from dots.toml or set a different dst.",
			}
		}
		return "already", nil
	}

	if dryRun {
		return "ok", nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}

	repoURL := expandURL(r.Repo)
	args := []string{"clone"}
	if r.Shallow {
		args = append(args, "--depth", "1")
	}
	args = append(args, repoURL, dst)

	if err := gitRun(args, ""); err != nil {
		return "", err
	}

	// Assert the pinned ref after cloning rather than via `git clone --branch`,
	// which rejects a raw commit SHA. syncRef handles tag/branch/SHA uniformly
	// and is the same path `update` uses.
	if !isLatestRef(r.Ref) {
		if err := syncRef(r, dst); err != nil {
			return "", err
		}
	}

	if r.OnInstall != "" {
		if err := shellRun(r.OnInstall, dst); err != nil {
			return "", err
		}
	}
	return "ok", nil
}

// syncRef brings the working tree in line with the configured ref. When the
// repo tracks the default branch (ref unset or "latest") it pulls the tip;
// otherwise it asserts the pinned tag/branch/SHA, resetting a tracked branch to
// its remote so a moved branch is honoured and detaching onto a tag or SHA.
func syncRef(r config.RepoEntry, dst string) error {
	if isLatestRef(r.Ref) {
		if r.Shallow {
			if err := gitRun([]string{"fetch", "--depth", "1"}, dst); err != nil {
				return err
			}
			return gitRun([]string{"reset", "--hard", "FETCH_HEAD"}, dst)
		}
		return gitRun([]string{"pull"}, dst)
	}

	if r.Shallow {
		if err := gitRun([]string{"fetch", "--depth", "1", "origin", r.Ref}, dst); err != nil {
			return err
		}
		return gitRun([]string{"reset", "--hard", "FETCH_HEAD"}, dst)
	}

	if err := gitRun([]string{"fetch", "--tags", "--force", "origin"}, dst); err != nil {
		return err
	}
	// A remote branch is reset to its tip so a moved branch is tracked; a tag or
	// SHA is checked out detached at exactly that commit.
	if remoteBranchExists(dst, r.Ref) {
		return gitRun([]string{"checkout", "-B", r.Ref, "origin/" + r.Ref}, dst)
	}
	return gitRun([]string{"checkout", "--force", r.Ref}, dst)
}

// remoteBranchExists reports whether origin/<ref> resolves to a branch.
func remoteBranchExists(dst, ref string) bool {
	_, err := gitOutput([]string{"rev-parse", "--verify", "--quiet", "refs/remotes/origin/" + ref}, dst)
	return err == nil
}

// repoState returns the current state of a repo without modifying it.
func repoState(r config.RepoEntry) (RepoState, error) {
	dst := fileutil.Expand(r.Dst)
	s := RepoState{Entry: r}
	if !isLatestRef(r.Ref) {
		s.Ref = r.Ref
	}

	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return s, nil
	}
	s.Exists = true

	// Current branch or SHA.
	if out, err := gitOutput([]string{"rev-parse", "--abbrev-ref", "HEAD"}, dst); err == nil {
		s.Current = strings.TrimSpace(out)
	}

	// Dirty check.
	if out, err := gitOutput([]string{"status", "--porcelain"}, dst); err == nil {
		s.Dirty = strings.TrimSpace(out) != ""
	}

	// Commits behind upstream (best-effort; fails gracefully if no upstream).
	if out, err := gitOutput([]string{"rev-list", "HEAD..@{u}", "--count"}, dst); err == nil {
		n := 0
		fmt.Sscanf(strings.TrimSpace(out), "%d", &n)
		s.Behind = n
	}

	// When a ref is pinned, report whether HEAD sits at it. Resolution is local
	// (no fetch) so status stays side-effect-free; an unresolvable ref simply
	// reads as off-target.
	if s.Ref != "" {
		s.OnTarget = headAtRef(dst, s.Ref)
	}

	return s, nil
}

// headAtRef reports whether HEAD resolves to the same commit as ref. It tries
// the ref as given and as a tag, so both branch and tag pins are recognised.
func headAtRef(dst, ref string) bool {
	head, err := gitOutput([]string{"rev-parse", "HEAD"}, dst)
	if err != nil {
		return false
	}
	for _, candidate := range []string{ref + "^{commit}", "refs/tags/" + ref + "^{commit}"} {
		if out, err := gitOutput([]string{"rev-parse", "--verify", "--quiet", candidate}, dst); err == nil {
			if strings.TrimSpace(out) == strings.TrimSpace(head) {
				return true
			}
		}
	}
	return false
}

// expandURL expands a GitHub shorthand "user/repo" to a full HTTPS URL.
func expandURL(repo string) string {
	if strings.Contains(repo, "/") && !strings.Contains(repo, "://") && !strings.Contains(repo, "@") {
		return "https://github.com/" + repo
	}
	return repo
}

// gitRun runs a git command in cwd (empty = inherit).
func gitRun(args []string, cwd string) error {
	cmd := exec.Command("git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return &errs.DotsError{
			Msg:  fmt.Sprintf("git %s failed", args[0]),
			Hint: strings.TrimSpace(string(out)),
		}
	}
	return nil
}

// gitOutput runs a git command and returns its stdout.
func gitOutput(args []string, cwd string) (string, error) {
	cmd := exec.Command("git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	out, err := cmd.Output()
	return string(out), err
}

// shellRun runs a shell command string in cwd.
func shellRun(command, cwd string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = cwd
	if out, err := cmd.CombinedOutput(); err != nil {
		return &errs.DotsError{
			Msg:  fmt.Sprintf("hook command failed: %s", command),
			Hint: strings.TrimSpace(string(out)),
		}
	}
	return nil
}
