// Package ghrelease is a small GitHub releases client: resolve a release (latest
// or by tag) and download a release asset. It is shared by tool installation
// (internal/tools) and binary self-upgrade (internal/selfupdate).
package ghrelease

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pkuehne/dots/internal/errs"
)

// httpClient is the shared HTTP client; replaced in tests.
var httpClient = &http.Client{Timeout: 30 * time.Second}

// APIBase is the GitHub API root URL; replaced in tests.
var APIBase = "https://api.github.com"

// Release is a GitHub release with its downloadable assets.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset is a single downloadable file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// GetLatestRelease fetches the most recent published release for repo
// (e.g. "owner/name").
func GetLatestRelease(repo string) (*Release, error) {
	return getRelease(repo, fmt.Sprintf("%s/repos/%s/releases/latest", APIBase, repo))
}

// GetReleaseByTag fetches a release pinned by tag. The tag is tried as given
// and, if not found, with a "v" prefix (version "1.2.3" matches both 1.2.3 and
// v1.2.3 tags).
func GetReleaseByTag(repo, version string) (*Release, error) {
	release, err := getRelease(repo, fmt.Sprintf("%s/repos/%s/releases/tags/%s", APIBase, repo, version))
	if err != nil && !strings.HasPrefix(version, "v") {
		if r2, err2 := getRelease(repo, fmt.Sprintf("%s/repos/%s/releases/tags/v%s", APIBase, repo, version)); err2 == nil {
			return r2, nil
		}
	}
	return release, err
}

func getRelease(repo, url string) (*Release, error) {
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

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, errs.NewTool(fmt.Sprintf("failed to parse GitHub API response for %s", repo), err.Error())
	}
	return &release, nil
}

// DownloadAsset streams the asset at url to the file dest.
func DownloadAsset(url, dest string) error {
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
