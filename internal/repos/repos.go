// Package repos clones and updates [[repo]] entries.
package repos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/fileutil"
	"github.com/pkuehne/dots/internal/platform"
)

// RepoState is the current state of a managed repo clone.
type RepoState struct {
	Entry   config.RepoEntry
	Exists  bool
	Dirty   bool
	Behind  int    // commits behind remote
	Current string // current ref (branch or SHA)
}

// Clone clones any repos that are not yet present on disk.
// If names is non-empty, only those repos are cloned.
func Clone(cfg config.Config, names []string, dryRun bool) error {
	for _, r := range active(cfg, names) {
		status, err := cloneOne(r, dryRun)
		if err != nil {
			return err
		}
		switch status {
		case "ok":
			if dryRun {
				fmt.Printf("  would clone %s → %s\n", r.Name, r.Dst)
			} else {
				fmt.Printf("  cloned %s → %s\n", r.Name, r.Dst)
			}
		}
	}
	return nil
}

// Update fetches and pulls repos that already exist.
func Update(cfg config.Config, names []string, dryRun bool) error {
	for _, r := range active(cfg, names) {
		status, err := updateOne(r, dryRun)
		if err != nil {
			return err
		}
		switch status {
		case "ok":
			if dryRun {
				fmt.Printf("  would update %s\n", r.Name)
			} else {
				fmt.Printf("  updated %s\n", r.Name)
			}
		case "missing":
			fmt.Printf("  skipped %s (not cloned — run 'dots repos clone')\n", r.Name)
		case "dirty":
			fmt.Printf("  skipped %s (uncommitted local changes — commit or stash first)\n", r.Name)
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
	if r.Ref != "" {
		args = append(args, "--branch", r.Ref)
	}
	args = append(args, repoURL, dst)

	if err := gitRun(args, ""); err != nil {
		return "", err
	}

	if r.OnInstall != "" {
		if err := shellRun(r.OnInstall, dst); err != nil {
			return "", err
		}
	}
	return "ok", nil
}

// updateOne updates a single repo. Returns "ok", "missing", "dirty", or an
// error.
func updateOne(r config.RepoEntry, dryRun bool) (string, error) {
	dst := fileutil.Expand(r.Dst)

	if _, err := os.Stat(dst); os.IsNotExist(err) {
		return "missing", nil
	}

	// A shallow update does `git reset --hard`, which would silently discard
	// local modifications. Refuse to update a dirty repo so user work survives.
	// (A normal `git pull` aborts on conflict on its own, so only shallow needs
	// the guard — but checking unconditionally keeps the behaviour uniform.)
	if out, err := gitOutput([]string{"status", "--porcelain"}, dst); err == nil {
		if strings.TrimSpace(out) != "" {
			return "dirty", nil
		}
	}

	if dryRun {
		return "ok", nil
	}

	if r.Shallow {
		if err := gitRun([]string{"fetch", "--depth", "1"}, dst); err != nil {
			return "", err
		}
		if err := gitRun([]string{"reset", "--hard", "FETCH_HEAD"}, dst); err != nil {
			return "", err
		}
	} else {
		if err := gitRun([]string{"pull"}, dst); err != nil {
			return "", err
		}
	}

	if r.OnUpdate != "" {
		if err := shellRun(r.OnUpdate, dst); err != nil {
			return "", err
		}
	}
	return "ok", nil
}

// repoState returns the current state of a repo without modifying it.
func repoState(r config.RepoEntry) (RepoState, error) {
	dst := fileutil.Expand(r.Dst)
	s := RepoState{Entry: r}

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

	return s, nil
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
