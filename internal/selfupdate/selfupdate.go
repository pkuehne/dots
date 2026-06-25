// Package selfupdate upgrades the running dots binary in place from its GitHub
// releases. Unlike [[tools]] installation it knows dots' own version (compiled
// in via main.version) and release-asset naming, and replaces the live binary
// atomically.
package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/fileutil"
	"github.com/pkuehne/dots/internal/ghrelease"
	"github.com/pkuehne/dots/internal/platform"
)

// Repo is the GitHub repository dots upgrades itself from.
const Repo = "pkuehne/dots"

// osExecutable resolves the running binary's path; overridden in tests.
var osExecutable = os.Executable

// Options controls a self-upgrade.
type Options struct {
	// CurrentVersion is the running binary's version (main.version). The
	// literal "dev" means a local build that cannot be compared to a release.
	CurrentVersion string
	// TargetVersion pins a specific release tag; empty means the latest release.
	TargetVersion string
	// Force upgrades even when the running version is already current or is a
	// dev build.
	Force bool
	// DryRun resolves the target release but writes nothing.
	DryRun bool
}

// Result describes what an upgrade resolved to.
type Result struct {
	CurrentVersion string // running version ("dev" for local builds)
	LatestVersion  string // resolved release tag (normalized to a leading "v")
	Available      bool   // a newer version is available (or Force is set)
	Applied        bool   // the binary was actually replaced
	BackupPath     string // path to the saved previous binary, when Applied
}

// AssetName returns the release asset for the current OS/arch, matching the
// naming used by install.sh and the release workflow (e.g. dots_linux_amd64).
func AssetName() string {
	return fmt.Sprintf("dots_%s_%s", platform.OSName(), platform.GoArch())
}

// normalize returns v with a leading "v" so semver can compare it. dots releases
// are tagged vX.Y.Z (release-please's component prefix is disabled), so the tag
// is also a valid version once normalized.
func normalize(v string) string {
	if v == "" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// resolve fetches the target release and fills the version-comparison fields of
// Result. It does not touch the binary.
func resolve(opts Options) (Result, *ghrelease.Release, error) {
	res := Result{CurrentVersion: opts.CurrentVersion}

	var release *ghrelease.Release
	var err error
	if opts.TargetVersion != "" {
		release, err = ghrelease.GetReleaseByTag(Repo, normalize(opts.TargetVersion))
	} else {
		release, err = ghrelease.GetLatestRelease(Repo)
	}
	if err != nil {
		return res, nil, err
	}
	res.LatestVersion = normalize(release.TagName)

	switch {
	case opts.Force:
		res.Available = true
	case !semver.IsValid(normalize(opts.CurrentVersion)):
		// "dev" or otherwise uncomparable: not available unless Force.
	case !semver.IsValid(res.LatestVersion):
		return res, nil, errs.New(
			fmt.Sprintf("release tag %q is not a valid version", release.TagName),
			"Pin a specific release with --version, or report this upstream.",
		)
	default:
		res.Available = semver.Compare(res.LatestVersion, normalize(opts.CurrentVersion)) > 0
	}
	return res, release, nil
}

// Check resolves the target release and reports whether an upgrade is available,
// without touching the binary.
func Check(opts Options) (Result, error) {
	res, _, err := resolve(opts)
	return res, err
}

// Run upgrades the running binary to the target release. When no upgrade is
// available it is a no-op (Applied is false). With DryRun it resolves the
// release but writes nothing.
func Run(opts Options) (Result, error) {
	res, release, err := resolve(opts)
	if err != nil {
		return res, err
	}
	if !res.Available {
		return res, nil
	}

	assetName := AssetName()
	var asset *ghrelease.Asset
	for i := range release.Assets {
		if release.Assets[i].Name == assetName {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil {
		names := make([]string, 0, len(release.Assets))
		for _, a := range release.Assets {
			names = append(names, a.Name)
		}
		return res, errs.New(
			fmt.Sprintf("no matching release asset %q in %s@%s", assetName, Repo, release.TagName),
			fmt.Sprintf("Available: %s\n\nYour platform may not have a prebuilt binary; build from source instead.",
				strings.Join(names, ", ")),
		)
	}

	if opts.DryRun {
		return res, nil
	}

	// Resolve the real path of the running binary (following symlinks) so we
	// replace the actual file, not a symlink to it.
	exe, err := osExecutable()
	if err != nil {
		return res, errs.New("cannot locate the running dots binary", err.Error())
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	// Download into the same directory as the target so the final rename stays
	// on one filesystem (rename across filesystems fails).
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".dots-upgrade-*")
	if err != nil {
		return res, errs.New(fmt.Sprintf("cannot create temp file in %s", dir),
			"dots needs write access to the directory containing its binary.\n"+
				err.Error())
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath) // no-op once renamed away

	if err := ghrelease.DownloadAsset(asset.BrowserDownloadURL, tmpPath, nil); err != nil {
		return res, err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return res, errs.New("cannot make the downloaded binary executable", err.Error())
	}

	// Back up the current binary for rollback, then atomically replace it. The
	// running process keeps executing from the old inode.
	backup, err := fileutil.Backup(exe)
	if err != nil {
		return res, err
	}
	if err := os.Rename(tmpPath, exe); err != nil {
		return res, errs.New(fmt.Sprintf("cannot replace %s", exe),
			fmt.Sprintf("The previous binary was saved to %s.\n%s", backup, err.Error()))
	}

	res.Applied = true
	res.BackupPath = backup
	return res, nil
}
