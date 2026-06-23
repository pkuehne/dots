// Package tools checks and installs [[tool]] entries using the method declared
// in each [[tool.install]] block (apt, github, brew, script, …).
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mholt/archives"
	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/fileutil"
	"github.com/pkuehne/dots/internal/platform"
)

// httpClient is the shared HTTP client; replaced in tests.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// githubAPIBase is the GitHub API root URL; replaced in tests.
var githubAPIBase = "https://api.github.com"

// CheckResult is the outcome of checking whether one tool is present.
type CheckResult struct {
	Tool      config.Tool
	Installed bool
}

// InstallOptions controls install behaviour.
type InstallOptions struct {
	DryRun bool
	Force  bool // reinstall even if already present
}

// ── Public API ────────────────────────────────────────────────────────────────

// Filter returns the subset of tools active on the given platform tags and
// profile that match any of the given names or the given tag. A tool with a
// non-empty Only is active only when Only intersects platforms; a tool with a
// non-empty Profile is active only when it equals profile. If names and tag
// are both empty, all active tools are returned.
func Filter(tools []config.Tool, names []string, tag string, platforms []string, profile string) []config.Tool {
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	var out []config.Tool
	for _, t := range tools {
		if len(t.Only) > 0 && !intersects(t.Only, platforms) {
			continue
		}
		if t.Profile != "" && t.Profile != profile {
			continue
		}
		if len(names) == 0 && tag == "" {
			out = append(out, t)
			continue
		}
		if nameSet[t.Name] {
			out = append(out, t)
			continue
		}
		if tag != "" {
			for _, tg := range t.Tags {
				if tg == tag {
					out = append(out, t)
					break
				}
			}
		}
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

// Check returns whether each tool is currently installed by running its check
// command (or falling back to exec.LookPath when check is empty).
func Check(tools []config.Tool) []CheckResult {
	results := make([]CheckResult, len(tools))
	for i, t := range tools {
		results[i] = CheckResult{Tool: t}
		results[i].Installed = toolIsInstalled(t)
	}
	return results
}

// Install installs tool using the best matching [[tool.install]] entry for the
// current platform. It is a no-op when the tool is already installed (unless
// opts.Force is set) or opts.DryRun is true.
func Install(tool config.Tool, cfg config.Config, plat, arch string, opts InstallOptions) error {
	if !opts.Force {
		if toolIsInstalled(tool) {
			return nil
		}
	}

	inst := findInstallMethod(tool, plat)
	if inst == nil {
		return errs.NewTool(
			fmt.Sprintf("no suitable install method found for %s on %s", tool.Name, plat),
			"Add an install method for your platform to dots.toml.",
		)
	}

	if opts.DryRun {
		return nil
	}

	binDir := cfg.ToolsConfig.BinDir
	if binDir == "" {
		binDir = "~/.local/bin"
	}
	binDir = fileutil.Expand(binDir)

	return installTool(tool, *inst, plat, binDir)
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func toolIsInstalled(tool config.Tool) bool {
	if tool.Check != "" {
		cmd := exec.Command("sh", "-c", tool.Check)
		return cmd.Run() == nil
	}
	_, err := exec.LookPath(tool.Name)
	return err == nil
}

// findInstallMethod selects the first [[tool.install]] entry whose method
// binary is available and whose only filter (if any) includes plat.
func findInstallMethod(tool config.Tool, plat string) *config.ToolInstall {
	available := map[string]bool{
		"github": true,
		"script": true,
		"manual": true,
	}
	for bin, key := range map[string]string{
		"pkg":     "pkg",
		"apt-get": "apt",
		"brew":    "brew",
		"cargo":   "cargo",
		"go":      "go",
		"pipx":    "pipx",
		"npm":     "npm",
	} {
		if _, err := exec.LookPath(bin); err == nil {
			available[key] = true
		}
	}
	for _, pip := range []string{"pip3", "pip"} {
		if _, err := exec.LookPath(pip); err == nil {
			available["pip"] = true
			break
		}
	}

	for i := range tool.Install {
		inst := &tool.Install[i]
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
		if available[inst.Method] {
			return inst
		}
	}
	return nil
}

func installTool(tool config.Tool, inst config.ToolInstall, plat, binDir string) error {
	switch inst.Method {
	case "pkg":
		return runCmd("pkg", "install", "-y", inst.Package)

	case "apt":
		if plat == "termux" {
			return errs.NewTool(
				"install method 'apt' requires sudo, which is not available on Termux",
				fmt.Sprintf("Use 'pkg' instead:\n  dots tools install %s", tool.Name),
			)
		}
		args := []string{"apt-get", "install", "-y", inst.Package}
		if os.Getuid() != 0 {
			args = append([]string{"sudo"}, args...)
		}
		return runCmd(args[0], args[1:]...)

	case "brew":
		return runCmd("brew", "install", inst.Package)

	case "cargo":
		if inst.Binary != "" {
			return runCmd("cargo", "install", inst.Package, "--bin", inst.Binary)
		}
		return runCmd("cargo", "install", inst.Package)

	case "go":
		pkg := inst.Package
		if !strings.HasSuffix(pkg, "@latest") {
			pkg += "@latest"
		}
		return runCmd("go", "install", pkg)

	case "pip":
		pip := "pip3"
		if _, err := exec.LookPath(pip); err != nil {
			pip = "pip"
		}
		return runCmd(pip, "install", "--user", inst.Package)

	case "pipx":
		return runCmd("pipx", "install", inst.Package)

	case "npm":
		return runCmd("npm", "install", "-g", inst.Package)

	case "github":
		return installGitHub(tool, inst, binDir)

	case "script":
		cmd := exec.Command("sh", "-c", inst.Script)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return errs.NewTool(fmt.Sprintf("script install failed for %s", tool.Name), err.Error())
		}
		return nil

	case "manual":
		note := inst.Note
		if note == "" {
			note = "see documentation"
		}
		fmt.Printf("  Manual install: %s\n", note)
		return nil

	default:
		return errs.NewTool(
			fmt.Sprintf("unknown install method: %s", inst.Method),
			"Supported: pkg, apt, brew, cargo, go, pip, pipx, npm, github, script, manual",
		)
	}
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return errs.NewTool(
			fmt.Sprintf("command failed: %s %s", name, strings.Join(args, " ")),
			err.Error(),
		)
	}
	return nil
}

// ── GitHub release install ────────────────────────────────────────────────────

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func githubGetLatestRelease(repo string) (*githubRelease, error) {
	return githubGetRelease(repo, fmt.Sprintf("%s/repos/%s/releases/latest", githubAPIBase, repo))
}

// githubGetReleaseByTag fetches the release pinned by a [[tool.install]]
// version field. The tag is tried as given and, if not found, with a "v"
// prefix (version = "1.2.3" matches both 1.2.3 and v1.2.3 tags).
func githubGetReleaseByTag(repo, version string) (*githubRelease, error) {
	release, err := githubGetRelease(repo, fmt.Sprintf("%s/repos/%s/releases/tags/%s", githubAPIBase, repo, version))
	if err != nil && !strings.HasPrefix(version, "v") {
		if r2, err2 := githubGetRelease(repo, fmt.Sprintf("%s/repos/%s/releases/tags/v%s", githubAPIBase, repo, version)); err2 == nil {
			return r2, nil
		}
	}
	return release, err
}

func githubGetRelease(repo, url string) (*githubRelease, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, errs.NewTool(fmt.Sprintf("cannot build GitHub API request for %s", repo), err.Error())
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errs.NewTool(
			fmt.Sprintf("failed to reach GitHub API for %s", repo),
			fmt.Sprintf("Network error: %v\n\nHints:\n"+
				"· Are you behind a proxy? Set: export HTTPS_PROXY=http://proxy:3128\n"+
				"· Check connectivity: curl https://api.github.com", err),
		)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusForbidden:
		return nil, errs.NewTool(
			fmt.Sprintf("GitHub API rate limit exceeded for %s", repo),
			"GitHub API rate limit exceeded (60 requests/hour for unauthenticated)\n\n"+
				"Set GITHUB_TOKEN to raise the limit to 5000 req/hour:\n  export GITHUB_TOKEN=ghp_...",
		)
	case http.StatusOK:
		// handled below
	default:
		return nil, errs.NewTool(
			fmt.Sprintf("GitHub API returned HTTP %d for %s", resp.StatusCode, repo),
			"Check the repository name and ensure a release exists.",
		)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, errs.NewTool(fmt.Sprintf("failed to parse GitHub API response for %s", repo), err.Error())
	}
	return &release, nil
}

func githubDownloadAsset(url, dest string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return errs.NewTool("cannot build asset download request", err.Error())
	}
	req.Header.Set("Accept", "application/octet-stream")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return errs.NewTool(fmt.Sprintf("failed to download asset from %s", url), err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errs.NewTool(
			fmt.Sprintf("asset download returned HTTP %d from %s", resp.StatusCode, url),
			"The download URL may be expired or the asset removed. Re-run to fetch the latest release.",
		)
	}

	f, err := os.Create(dest)
	if err != nil {
		return errs.NewTool("cannot create download destination", err.Error())
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return errs.NewTool("failed to write downloaded asset", err.Error())
	}
	return nil
}

func installGitHub(tool config.Tool, inst config.ToolInstall, binDir string) error {
	var release *githubRelease
	var err error
	if inst.Version != "" {
		release, err = githubGetReleaseByTag(inst.Repo, inst.Version)
	} else {
		release, err = githubGetLatestRelease(inst.Repo)
	}
	if err != nil {
		return err
	}

	version := strings.TrimPrefix(release.TagName, "v")
	arch := platform.Arch()
	osName := platform.OSName()
	goArch := platform.GoArch()

	// Apply per-tool arch_map override
	if mapped, ok := inst.ArchMap[arch]; ok {
		arch = mapped
	}

	// Expand token placeholders in the asset pattern
	assetPattern := inst.Asset
	if assetPattern == "" {
		assetPattern = fmt.Sprintf("%s-%s-*", tool.Name, version)
	}
	replacer := strings.NewReplacer(
		"{version}", version,
		"{arch}", arch,
		"{os}", osName,
		"{goarch}", goArch,
		"{name}", tool.Name,
	)
	assetPattern = replacer.Replace(assetPattern)

	// Find a matching asset
	var matched *githubAsset
	for i := range release.Assets {
		if globMatch(assetPattern, release.Assets[i].Name) {
			matched = &release.Assets[i]
			break
		}
	}
	if matched == nil {
		names := make([]string, 0, len(release.Assets))
		for _, a := range release.Assets {
			names = append(names, a.Name)
		}
		avail := strings.Join(names, ", ")
		if len(names) > 10 {
			avail = strings.Join(names[:10], ", ") + ", ..."
		}
		return errs.NewTool(
			fmt.Sprintf("no matching asset for %s in %s@%s", tool.Name, inst.Repo, release.TagName),
			fmt.Sprintf("Pattern: %s\nAvailable: %s", assetPattern, avail),
		)
	}

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return errs.NewTool(fmt.Sprintf("cannot create bin dir %s", binDir), err.Error())
	}

	tmpDir, err := os.MkdirTemp("", "dots-install-*")
	if err != nil {
		return errs.NewTool("cannot create temp directory", err.Error())
	}
	defer os.RemoveAll(tmpDir)

	downloadPath := filepath.Join(tmpDir, matched.Name)
	if err := githubDownloadAsset(matched.BrowserDownloadURL, downloadPath); err != nil {
		return err
	}

	binaryName := inst.Binary
	if binaryName == "" {
		binaryName = tool.Name
	}
	dest := filepath.Join(binDir, binaryName)

	if isExtractableArchive(matched.Name) {
		extractDir := filepath.Join(tmpDir, "extracted")
		if err := extractArchive(downloadPath, extractDir); err != nil {
			return err
		}
		return findAndInstallBinary(extractDir, binaryName, dest, inst.BinaryPath)
	}
	return installBinaryFile(downloadPath, dest)
}

// ── Archive extraction ────────────────────────────────────────────────────────

const (
	traversalHint = "The archive contains a path traversal entry. This may be a malicious archive."
	symlinkHint   = "The archive contains a symlink pointing outside the archive. This may be a malicious archive."
)

// archiveExtensions lists the suffixes dots will route to extractArchive. The
// mholt/archives library can decompress all of these uniformly; anything else
// (a bare binary, a lone .gz, …) is installed verbatim.
var archiveExtensions = []string{
	".tar.gz", ".tgz",
	".tar.bz2", ".tbz", ".tbz2",
	".tar.xz", ".txz",
	".tar.zst", ".tzst",
	".tar",
	".zip",
}

// isExtractableArchive reports whether name has a suffix dots knows how to
// extract a binary from.
func isExtractableArchive(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range archiveExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// containedIn reports whether p is dest itself or a path nested within dest.
// Both are cleaned, so it is robust against "." and trailing-separator noise.
func containedIn(dest, p string) bool {
	clean := filepath.Clean(dest)
	return p == clean || strings.HasPrefix(p, clean+string(os.PathSeparator))
}

// sanitizeArchivePath joins an archive entry name onto dest and verifies the
// resolved path stays within dest, returning the validated member path. It
// rejects entries containing ".." (a Zip Slip attack) by checking that the
// cleaned join is still rooted at dest. Returning the sanitized value (rather
// than a boolean) keeps the taint flow from check to use explicit.
func sanitizeArchivePath(dest, name string) (string, error) {
	memberPath := filepath.Join(dest, filepath.FromSlash(name))
	if !containedIn(dest, memberPath) {
		return "", errs.NewTool(
			fmt.Sprintf("refusing to extract %q — path escapes target", name),
			traversalHint,
		)
	}
	return memberPath, nil
}

// extractArchive extracts the archive at archivePath into dest using the
// mholt/archives library, which handles tar.gz, tar.bz2, tar.xz, tar.zst, tar
// and zip uniformly. Directories, regular files and symlinks are materialised;
// every entry's path is checked to stay within dest, and symlinks are rejected
// if their target escapes dest.
func extractArchive(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return errs.NewTool(fmt.Sprintf("cannot open archive %s", archivePath), err.Error())
	}
	defer f.Close()

	ctx := context.Background()
	format, stream, err := archives.Identify(ctx, filepath.Base(archivePath), f)
	if err != nil {
		return errs.NewTool(fmt.Sprintf("cannot identify archive format of %s", archivePath), err.Error())
	}
	extractor, ok := format.(archives.Extractor)
	if !ok {
		return errs.NewTool(
			fmt.Sprintf("asset %q is not an extractable archive", filepath.Base(archivePath)),
			"Use the 'asset' field to select a supported archive (.tar.gz, .tar.xz, .tar.bz2, .tar.zst, .zip).",
		)
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return errs.NewTool("cannot create extraction directory", err.Error())
	}

	handler := func(ctx context.Context, info archives.FileInfo) error {
		memberPath, err := sanitizeArchivePath(dest, info.NameInArchive)
		if err != nil {
			return err
		}

		switch {
		case info.IsDir():
			if err := os.MkdirAll(memberPath, 0o755); err != nil {
				return errs.NewTool("cannot create directory from archive", err.Error())
			}
			return nil

		case info.Mode()&fs.ModeSymlink != 0:
			return extractSymlink(dest, memberPath, info)

		default:
			return extractRegularFile(memberPath, info)
		}
	}

	if err := extractor.Extract(ctx, stream, handler); err != nil {
		// errs raised inside the handler already carry a hint; pass them
		// through unchanged rather than wrapping them again (the library may
		// wrap the handler error, so unwrap with errors.As).
		var te *errs.ToolInstallError
		if errors.As(err, &te) {
			return te
		}
		return errs.NewTool(fmt.Sprintf("failed to extract archive %s", filepath.Base(archivePath)), err.Error())
	}
	return nil
}

// extractSymlink materialises an in-archive symlink at memberPath, rejecting
// targets that resolve outside dest.
func extractSymlink(dest, memberPath string, info archives.FileInfo) error {
	target := info.LinkTarget
	if filepath.IsAbs(target) {
		return errs.NewTool(
			fmt.Sprintf("refusing to extract symlink %q → %q — absolute target", info.NameInArchive, target),
			symlinkHint,
		)
	}
	// Resolve the target relative to the link's own directory and ensure it
	// stays within dest.
	resolved := filepath.Join(filepath.Dir(memberPath), filepath.FromSlash(target))
	if !containedIn(dest, resolved) {
		return errs.NewTool(
			fmt.Sprintf("refusing to extract symlink %q → %q — target escapes target directory", info.NameInArchive, target),
			symlinkHint,
		)
	}
	if err := os.MkdirAll(filepath.Dir(memberPath), 0o755); err != nil {
		return errs.NewTool("cannot create parent directory", err.Error())
	}
	// Replace any existing entry so extraction is idempotent.
	_ = os.Remove(memberPath)
	if err := os.Symlink(target, memberPath); err != nil {
		return errs.NewTool("cannot create symlink from archive", err.Error())
	}
	return nil
}

// extractRegularFile writes an in-archive regular file to memberPath.
func extractRegularFile(memberPath string, info archives.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(memberPath), 0o755); err != nil {
		return errs.NewTool("cannot create parent directory", err.Error())
	}
	rc, err := info.Open()
	if err != nil {
		return errs.NewTool("cannot read archive entry", err.Error())
	}
	defer rc.Close()

	out, err := os.OpenFile(memberPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return errs.NewTool("cannot create extracted file", err.Error())
	}
	_, copyErr := io.Copy(out, rc)
	closeErr := out.Close()
	if copyErr != nil {
		return errs.NewTool("cannot write extracted file", copyErr.Error())
	}
	if closeErr != nil {
		return errs.NewTool("cannot write extracted file", closeErr.Error())
	}
	return nil
}

// findAndInstallBinary locates and installs a binary from an extracted archive.
// When binaryPath is set it is used as an exact relative path inside extractDir.
// Otherwise the shallowest file matching binaryName is chosen.
func findAndInstallBinary(extractDir, binaryName, dest, binaryPath string) error {
	if binaryPath != "" {
		src := filepath.Join(extractDir, filepath.FromSlash(binaryPath))
		info, err := os.Stat(src)
		if err != nil || !info.Mode().IsRegular() {
			return errs.NewTool(
				fmt.Sprintf("binary_path %q not found in archive", binaryPath),
				"Check the 'binary_path' field in the install method.",
			)
		}
		return installBinaryFile(src, dest)
	}

	// Walk and collect all files whose base name equals binaryName.
	var candidates []string
	_ = filepath.WalkDir(extractDir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == binaryName {
			candidates = append(candidates, path)
		}
		return nil
	})

	if len(candidates) == 0 {
		return errs.NewTool(
			fmt.Sprintf("binary %q not found in archive", binaryName),
			"Check the 'binary' field in the install method.",
		)
	}

	// Prefer the shallowest path (fewest separators).
	best := candidates[0]
	for _, c := range candidates[1:] {
		if strings.Count(c, string(filepath.Separator)) < strings.Count(best, string(filepath.Separator)) {
			best = c
		}
	}
	return installBinaryFile(best, dest)
}

func installBinaryFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return errs.NewTool(fmt.Sprintf("cannot read binary from %s", src), err.Error())
	}
	if err := os.WriteFile(dest, data, 0o755); err != nil {
		return errs.NewTool(fmt.Sprintf("cannot install binary to %s", dest), err.Error())
	}
	return nil
}

// ── Misc helpers ──────────────────────────────────────────────────────────────

// globMatch returns true when name matches the shell-glob pattern (*, ? supported).
func globMatch(pattern, name string) bool {
	re := "^" + strings.NewReplacer(
		`\*`, `.*`,
		`\?`, `.`,
	).Replace(regexp.QuoteMeta(pattern)) + "$"
	matched, _ := regexp.MatchString(re, name)
	return matched
}
