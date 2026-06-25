package tools

import (
	"strings"
	"sync"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/lockfile"
	"github.com/pkuehne/dots/internal/parallel"
	"github.com/pkuehne/dots/internal/platform"
	"github.com/pkuehne/dots/internal/ui"
)

// TrackState classifies how a tool's installed version compares to its target.
type TrackState int

const (
	// NotTracked means the tool has no github install method, so dots cannot
	// assert a version (package managers track their own versions).
	NotTracked TrackState = iota
	// NotInstalled means the tool is version-tracked but is not currently
	// installed (no binary, or no recorded version to compare).
	NotInstalled
	// UpToDate means the installed version matches the target.
	UpToDate
	// Outdated means the installed version differs from the target.
	Outdated
	// Unknown means the target could not be resolved (e.g. network error while
	// querying the latest release).
	Unknown
)

// VersionState is the version-tracking status of one tool.
type VersionState struct {
	Tool      config.Tool
	State     TrackState
	Installed string // version recorded in the lockfile ("" if none)
	Target    string // resolved target version ("" if unresolved)
	Pinned    bool   // target is an explicit pin rather than "latest"
	Err       error  // set when State is Unknown
}

// githubInstall returns the github install method active for plat, or nil when
// the tool has none. Unlike findInstallMethod it does not require any binary to
// be present — github installs are always available — so it can classify a
// tool's tracking status even before anything is installed.
func githubInstall(tool config.Tool, plat string) *config.ToolInstall {
	for i := range tool.Install {
		inst := &tool.Install[i]
		if inst.Method != "github" {
			continue
		}
		if len(inst.Only) > 0 {
			match := false
			for _, o := range inst.Only {
				if o == plat {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		return inst
	}
	return nil
}

// targetVersion resolves the version a github install method asserts: the
// pinned tag (without a leading "v") when set, otherwise the latest release.
// The boolean reports whether the target is an explicit pin.
func targetVersion(inst config.ToolInstall) (version string, pinned bool, err error) {
	if !isLatest(inst.Version) {
		return strings.TrimPrefix(inst.Version, "v"), true, nil
	}
	release, err := resolveRelease(inst)
	if err != nil {
		return "", false, err
	}
	return strings.TrimPrefix(release.TagName, "v"), false, nil
}

// VersionStatus reports the version-tracking status of each tool. Targets for
// pinned tools are read straight from config; "latest" tools require a GitHub
// API call to resolve the newest release, and a failure there is recorded on
// the individual VersionState rather than aborting the whole report.
func VersionStatus(toolList []config.Tool, plat string, lock *lockfile.Lock) []VersionState {
	out := make([]VersionState, 0, len(toolList))
	for _, t := range toolList {
		vs := VersionState{Tool: t}
		inst := githubInstall(t, plat)
		if inst == nil {
			vs.State = NotTracked
			out = append(out, vs)
			continue
		}

		if entry, ok := lock.Get(t.Name); ok {
			vs.Installed = entry.Version
		}

		target, pinned, err := targetVersion(*inst)
		vs.Pinned = pinned
		if err != nil {
			vs.State = Unknown
			vs.Err = err
			out = append(out, vs)
			continue
		}
		vs.Target = target

		switch {
		case vs.Installed == "" || !toolIsInstalled(t):
			vs.State = NotInstalled
		case vs.Installed == target:
			vs.State = UpToDate
		default:
			vs.State = Outdated
		}
		out = append(out, vs)
	}
	return out
}

// UpdateResult is the outcome of processing one tool during `tools update`.
type UpdateResult struct {
	Tool   config.Tool
	Action string // "updated", "installed", "uptodate", "untracked", "failed", or a dry-run "would-update"/"would-install"
	From   string // previous version (for updates)
	To     string // new/target version
	Err    error  // set when Action is "failed"
}

// DefaultJobs is the number of tools updated/installed concurrently when the
// caller does not specify. The work is network-bound (a GitHub API call plus a
// download per tool), so a handful of workers hides most of the latency without
// hammering the API.
const DefaultJobs = 4

// Update brings version-tracked (github) tools in line with their target
// version, reinstalling any whose recorded version differs from the target (or
// that are not installed). Tools are processed concurrently (up to jobs at a
// time), each reporting live progress through prog. Non-github tools are
// reported as "untracked" and left to their package manager. Results are
// returned in config order regardless of completion order; the caller persists
// lock with Lock.Save(). The returned error is the first failure encountered
// (other tools still run); per-tool failures are also recorded on their result.
func Update(cfg config.Config, names []string, tag, plat, arch string, lock *lockfile.Lock, dryRun bool, prog ui.Progress, jobs int) ([]UpdateResult, error) {
	active := Filter(cfg.Tools, names, tag, platform.Platforms(), cfg.ActiveProfile)
	results := make([]UpdateResult, len(active))

	var errMu sync.Mutex
	var firstErr error
	recordErr := func(err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}

	parallel.Run(active, jobs, func(i int, t config.Tool) {
		inst := githubInstall(t, plat)
		if inst == nil {
			// Untracked tools need no work and no GitHub call; record inline and
			// open no task — they are left to their package manager.
			results[i] = UpdateResult{Tool: t, Action: "untracked"}
			return
		}
		results[i] = updateTool(cfg, t, *inst, plat, arch, lock, dryRun, prog, recordErr)
	})
	return results, firstErr
}

// updateTool resolves one tool's target, decides whether it needs work, and
// (outside dry-run) installs it, driving a progress task through its stages.
func updateTool(cfg config.Config, t config.Tool, inst config.ToolInstall, plat, arch string, lock *lockfile.Lock, dryRun bool, prog ui.Progress, recordErr func(error)) UpdateResult {
	// Resolving a "latest" target is a GitHub round-trip. In dry-run we surface a
	// transient "resolving" row so the wait isn't silent; the resolved row clears
	// before the command prints its predicted-action table. The live (install)
	// path lets Install drive the bar through its own stages instead.
	var resolveTask ui.Task
	if dryRun {
		resolveTask = prog.Task(t.Name)
		resolveTask.Stage("resolving")
	}
	target, _, err := targetVersion(inst)
	if resolveTask != nil {
		// Clear the transient row in either case; the predicted-action table is
		// the single source of truth, including for resolve failures.
		resolveTask.Done("")
	}
	if err != nil {
		recordErr(err)
		return UpdateResult{Tool: t, Action: "failed", Err: err}
	}

	var installed string
	if entry, ok := lock.Get(t.Name); ok {
		installed = entry.Version
	}
	present := toolIsInstalled(t)

	if installed == target && present {
		return UpdateResult{Tool: t, Action: "uptodate", To: target}
	}

	action, would := "updated", "would-update"
	if !present || installed == "" {
		action, would = "installed", "would-install"
	}
	if dryRun {
		return UpdateResult{Tool: t, Action: would, From: installed, To: target}
	}

	task := prog.Task(t.Name)
	opts := InstallOptions{Force: true, Lock: lock, Task: task}
	if err := Install(t, cfg, plat, arch, opts); err != nil {
		task.Fail(err)
		recordErr(err)
		return UpdateResult{Tool: t, Action: "failed", From: installed, To: target, Err: err}
	}

	detail := target
	if action == "updated" {
		detail = installed + " → " + target
	}
	task.Done(detail)
	return UpdateResult{Tool: t, Action: action, From: installed, To: target}
}
