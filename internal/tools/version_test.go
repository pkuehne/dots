package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/ghrelease"
	"github.com/pkuehne/dots/internal/lockfile"
)

// newVersionServer serves a fake release whose latest tag is latestTag and which
// offers a single asset of the given name/content. It restores APIBase on
// cleanup. A nil content still serves the metadata (enough for status checks).
func newVersionServer(t *testing.T, latestTag, assetName string, assetContent []byte) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/latest"),
			strings.Contains(r.URL.Path, "/releases/tags/"):
			rel := ghrelease.Release{TagName: latestTag}
			if assetName != "" {
				rel.Assets = []ghrelease.Asset{{
					Name:               assetName,
					BrowserDownloadURL: srv.URL + "/download/" + assetName,
				}}
			}
			_ = json.NewEncoder(w).Encode(rel)
		case strings.Contains(r.URL.Path, "/download/"):
			w.Write(assetContent)
		default:
			http.NotFound(w, r)
		}
	}))
	orig := ghrelease.APIBase
	ghrelease.APIBase = srv.URL
	t.Cleanup(func() {
		srv.Close()
		ghrelease.APIBase = orig
	})
}

func TestIsLatest(t *testing.T) {
	for _, v := range []string{"", "latest", "LATEST"} {
		if !isLatest(v) {
			t.Errorf("isLatest(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"1.2.3", "v1.2.3"} {
		if isLatest(v) {
			t.Errorf("isLatest(%q) = true, want false", v)
		}
	}
}

func TestGithubInstall(t *testing.T) {
	tool := config.Tool{Install: []config.ToolInstall{
		{Method: "apt", Package: "rg"},
		{Method: "github", Repo: "x/y", Only: []string{"darwin"}},
		{Method: "github", Repo: "a/b"},
	}}
	got := githubInstall(tool, "linux")
	if got == nil || got.Repo != "a/b" {
		t.Fatalf("want github a/b on linux, got %+v", got)
	}
	got = githubInstall(tool, "darwin")
	if got == nil || got.Repo != "x/y" {
		t.Fatalf("want github x/y on darwin, got %+v", got)
	}
	if githubInstall(config.Tool{Install: []config.ToolInstall{{Method: "apt"}}}, "linux") != nil {
		t.Error("apt-only tool should not be github-tracked")
	}
}

func TestTargetVersion_PinnedNoNetwork(t *testing.T) {
	// No server configured; a pinned version must resolve without any HTTP call.
	v, pinned, err := targetVersion(config.ToolInstall{Method: "github", Repo: "a/b", Version: "v2.3.4"})
	if err != nil {
		t.Fatalf("targetVersion pinned: %v", err)
	}
	if v != "2.3.4" || !pinned {
		t.Errorf("got (%q, %v), want (2.3.4, true)", v, pinned)
	}
}

func TestVersionStatus_States(t *testing.T) {
	newVersionServer(t, "v1.0.0", "", nil)

	lock, _ := lockfile.Load(filepath.Join(t.TempDir(), "lock.toml"))
	lock.Set("uptodate", lockfile.Entry{Version: "1.0.0"})
	lock.Set("outdated", lockfile.Entry{Version: "0.9.0"})
	lock.Set("missing-bin", lockfile.Entry{Version: "1.0.0"})

	toolList := []config.Tool{
		{Name: "uptodate", Check: "true", Install: []config.ToolInstall{{Method: "github", Repo: "a/b"}}},
		{Name: "outdated", Check: "true", Install: []config.ToolInstall{{Method: "github", Repo: "a/b"}}},
		{Name: "missing-bin", Check: "false", Install: []config.ToolInstall{{Method: "github", Repo: "a/b"}}},
		{Name: "never-installed", Check: "true", Install: []config.ToolInstall{{Method: "github", Repo: "a/b"}}},
		{Name: "apt-tool", Install: []config.ToolInstall{{Method: "apt", Package: "x"}}},
	}

	states := VersionStatus(toolList, "linux", lock)
	want := map[string]TrackState{
		"uptodate":        UpToDate,
		"outdated":        Outdated,
		"missing-bin":     NotInstalled,
		"never-installed": NotInstalled,
		"apt-tool":        NotTracked,
	}
	for _, s := range states {
		if want[s.Tool.Name] != s.State {
			t.Errorf("%s: state = %v, want %v", s.Tool.Name, s.State, want[s.Tool.Name])
		}
	}
}

func TestVersionStatus_UnknownOnResolveError(t *testing.T) {
	// Point APIBase at a server that 404s every request so latest cannot resolve.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	orig := ghrelease.APIBase
	ghrelease.APIBase = srv.URL
	t.Cleanup(func() { srv.Close(); ghrelease.APIBase = orig })

	lock, _ := lockfile.Load(filepath.Join(t.TempDir(), "lock.toml"))
	toolList := []config.Tool{
		{Name: "t", Check: "true", Install: []config.ToolInstall{{Method: "github", Repo: "a/b"}}},
	}
	states := VersionStatus(toolList, "linux", lock)
	if states[0].State != Unknown || states[0].Err == nil {
		t.Fatalf("want Unknown with error, got state=%v err=%v", states[0].State, states[0].Err)
	}
}

func TestUpdate_ReinstallsOutdated(t *testing.T) {
	asset := "rg_1.0.0_Linux_x86_64.tar.gz"
	data := makeTarGz(t,
		[]struct{ name, content string }{{"rg", "#!/bin/sh\necho rg"}}, nil)
	newVersionServer(t, "v1.0.0", asset, data)

	binDir := t.TempDir()
	lock, _ := lockfile.Load(filepath.Join(t.TempDir(), "lock.toml"))
	lock.Set("rg", lockfile.Entry{Version: "0.9.0"})

	cfg := config.Config{
		ToolsConfig: config.ToolsConfig{BinDir: binDir},
		Tools: []config.Tool{{
			Name:  "rg",
			Check: "true", // pretend the binary is present
			Install: []config.ToolInstall{{
				Method: "github",
				Repo:   "a/b",
				Asset:  "rg_{version}_Linux_x86_64.tar.gz",
				Binary: "rg",
			}},
		}},
	}

	results, err := Update(cfg, nil, "", "linux", "x86_64", lock, false)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(results) != 1 || results[0].Action != "updated" {
		t.Fatalf("want one 'updated' result, got %+v", results)
	}
	if results[0].From != "0.9.0" || results[0].To != "1.0.0" {
		t.Errorf("version transition = %s → %s, want 0.9.0 → 1.0.0", results[0].From, results[0].To)
	}
	if e, _ := lock.Get("rg"); e.Version != "1.0.0" {
		t.Errorf("lockfile not updated: %q", e.Version)
	}
}

func TestUpdate_DryRunNoChange(t *testing.T) {
	newVersionServer(t, "v1.0.0", "", nil)
	lock, _ := lockfile.Load(filepath.Join(t.TempDir(), "lock.toml"))
	lock.Set("rg", lockfile.Entry{Version: "0.9.0"})

	cfg := config.Config{Tools: []config.Tool{{
		Name: "rg", Check: "true",
		Install: []config.ToolInstall{{Method: "github", Repo: "a/b"}},
	}}}

	results, err := Update(cfg, nil, "", "linux", "x86_64", lock, true)
	if err != nil {
		t.Fatalf("Update dry-run: %v", err)
	}
	if results[0].Action != "would-update" {
		t.Errorf("action = %q, want would-update", results[0].Action)
	}
	if e, _ := lock.Get("rg"); e.Version != "0.9.0" {
		t.Errorf("dry-run must not mutate lock, got %q", e.Version)
	}
}

func TestUpdate_Untracked(t *testing.T) {
	lock, _ := lockfile.Load(filepath.Join(t.TempDir(), "lock.toml"))
	cfg := config.Config{Tools: []config.Tool{{
		Name: "rg", Install: []config.ToolInstall{{Method: "apt", Package: "rg"}},
	}}}
	results, err := Update(cfg, nil, "", "linux", "x86_64", lock, false)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(results) != 1 || results[0].Action != "untracked" {
		t.Fatalf("want untracked, got %+v", results)
	}
}
