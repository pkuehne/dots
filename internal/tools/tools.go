// Package tools checks and installs [[tool]] entries using the method declared
// in each [[tool.install]] block (apt, github, brew, script, …).
package tools

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/errs"
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
	Version   string
}

// InstallOptions controls install behaviour.
type InstallOptions struct {
	DryRun bool
	Force  bool // reinstall even if already present
}

// ── Public API ────────────────────────────────────────────────────────────────

// Filter returns the subset of tools matching any of the given names or the
// given tag. If both are empty, all tools are returned.
func Filter(tools []config.Tool, names []string, tag string) []config.Tool {
	if len(names) == 0 && tag == "" {
		return tools
	}
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	var out []config.Tool
	for _, t := range tools {
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

// Check returns whether each tool is currently installed by running its check
// command (or falling back to exec.LookPath when check is empty).
func Check(tools []config.Tool, plat, arch string) []CheckResult {
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
	binDir = expandHome(binDir)

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
	url := fmt.Sprintf("%s/repos/%s/releases/latest", githubAPIBase, repo)
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
	release, err := githubGetLatestRelease(inst.Repo)
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

	name := matched.Name
	switch {
	case strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz"):
		extractDir := filepath.Join(tmpDir, "extracted")
		if err := safeTarExtractAll(downloadPath, extractDir); err != nil {
			return err
		}
		return findAndInstallBinary(extractDir, binaryName, dest, inst.BinaryPath)
	case strings.HasSuffix(name, ".zip"):
		extractDir := filepath.Join(tmpDir, "extracted")
		if err := safeZipExtractAll(downloadPath, extractDir); err != nil {
			return err
		}
		return findAndInstallBinary(extractDir, binaryName, dest, inst.BinaryPath)
	default:
		return installBinaryFile(downloadPath, dest)
	}
}

// ── Archive extraction ────────────────────────────────────────────────────────

const (
	traversalHint = "The archive contains a path traversal entry. This may be a malicious archive."
	symlinkHint   = "The archive contains an absolute symlink. This may be a malicious archive."
)

// pathEscapes reports whether memberPath would resolve outside dest.
func pathEscapes(memberPath, dest string) bool {
	destAbs, _ := filepath.Abs(dest)
	memberAbs, _ := filepath.Abs(memberPath)
	prefix := destAbs + string(filepath.Separator)
	return !strings.HasPrefix(memberAbs, prefix) && memberAbs != destAbs
}

// safeTarExtractAll extracts a .tar.gz archive into dest, rejecting any entry
// whose resolved path escapes dest or that contains an absolute symlink target.
func safeTarExtractAll(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return errs.NewTool(fmt.Sprintf("cannot open archive %s", archivePath), err.Error())
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return errs.NewTool("cannot decompress archive", err.Error())
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return errs.NewTool("cannot create extraction directory", err.Error())
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errs.NewTool("failed to read tar archive", err.Error())
		}

		memberPath := filepath.Join(dest, filepath.FromSlash(hdr.Name))
		if pathEscapes(memberPath, dest) {
			return errs.NewTool(
				fmt.Sprintf("refusing to extract %q — path escapes target", hdr.Name),
				traversalHint,
			)
		}

		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			if filepath.IsAbs(hdr.Linkname) {
				return errs.NewTool(
					fmt.Sprintf("refusing to extract symlink %q → %q — absolute target", hdr.Name, hdr.Linkname),
					symlinkHint,
				)
			}
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(memberPath, 0o755); err != nil {
				return errs.NewTool("cannot create directory from archive", err.Error())
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(memberPath), 0o755); err != nil {
				return errs.NewTool("cannot create parent directory", err.Error())
			}
			out, err := os.OpenFile(memberPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, hdr.FileInfo().Mode())
			if err != nil {
				return errs.NewTool("cannot create extracted file", err.Error())
			}
			_, copyErr := io.Copy(out, tr)
			out.Close()
			if copyErr != nil {
				return errs.NewTool("cannot write extracted file", copyErr.Error())
			}
		}
	}
	return nil
}

// safeZipExtractAll extracts a .zip archive into dest, rejecting any entry
// whose resolved path escapes dest.
func safeZipExtractAll(archivePath, dest string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return errs.NewTool(fmt.Sprintf("cannot open zip archive %s", archivePath), err.Error())
	}
	defer r.Close()

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return errs.NewTool("cannot create extraction directory", err.Error())
	}

	for _, f := range r.File {
		memberPath := filepath.Join(dest, filepath.FromSlash(f.Name))
		if pathEscapes(memberPath, dest) {
			return errs.NewTool(
				fmt.Sprintf("refusing to extract %q — path escapes target", f.Name),
				traversalHint,
			)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(memberPath, 0o755); err != nil {
				return errs.NewTool("cannot create directory from archive", err.Error())
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(memberPath), 0o755); err != nil {
			return errs.NewTool("cannot create parent directory", err.Error())
		}

		out, err := os.OpenFile(memberPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			return errs.NewTool("cannot create extracted file", err.Error())
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return errs.NewTool("cannot read from zip entry", err.Error())
		}
		_, copyErr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if copyErr != nil {
			return errs.NewTool("cannot write extracted file", copyErr.Error())
		}
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

func expandHome(path string) string {
	if path == "~" {
		if h, err := os.UserHomeDir(); err == nil {
			return h
		}
	}
	if strings.HasPrefix(path, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, path[2:])
		}
	}
	return path
}
