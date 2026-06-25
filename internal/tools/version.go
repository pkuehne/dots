package tools

import (
	"strings"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/lockfile"
	"github.com/pkuehne/dots/internal/platform"
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
	Action string // "updated", "installed", "uptodate", "untracked", or a dry-run "would-update"/"would-install"
	From   string // previous version (for updates)
	To     string // new/target version
}

// Update brings version-tracked (github) tools in line with their target
// version, reinstalling any whose recorded version differs from the target (or
// that are not installed). Non-github tools are reported as "untracked" and
// left to their package manager. The caller persists lock with Lock.Save().
func Update(cfg config.Config, names []string, tag, plat, arch string, lock *lockfile.Lock, dryRun bool) ([]UpdateResult, error) {
	active := Filter(cfg.Tools, names, tag, platform.Platforms(), cfg.ActiveProfile)
	results := make([]UpdateResult, 0, len(active))

	for _, t := range active {
		inst := githubInstall(t, plat)
		if inst == nil {
			results = append(results, UpdateResult{Tool: t, Action: "untracked"})
			continue
		}

		target, _, err := targetVersion(*inst)
		if err != nil {
			return results, err
		}

		var installed string
		if entry, ok := lock.Get(t.Name); ok {
			installed = entry.Version
		}
		present := toolIsInstalled(t)

		if installed == target && present {
			results = append(results, UpdateResult{Tool: t, Action: "uptodate", To: target})
			continue
		}

		action := "updated"
		would := "would-update"
		if !present || installed == "" {
			action, would = "installed", "would-install"
		}
		if dryRun {
			results = append(results, UpdateResult{Tool: t, Action: would, From: installed, To: target})
			continue
		}

		opts := InstallOptions{Force: true, Lock: lock}
		if err := Install(t, cfg, plat, arch, opts); err != nil {
			return results, err
		}
		results = append(results, UpdateResult{Tool: t, Action: action, From: installed, To: target})
	}
	return results, nil
}
