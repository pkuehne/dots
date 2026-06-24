package selfupdate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkuehne/dots/internal/ghrelease"
)

// fakeRelease serves a release with the given tag and assets, pointing
// ghrelease.APIBase at itself for the test.
func fakeRelease(t *testing.T, tag string, assets map[string][]byte) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/latest"),
			strings.Contains(r.URL.Path, "/releases/tags/"):
			rel := ghrelease.Release{TagName: tag}
			for name := range assets {
				rel.Assets = append(rel.Assets, ghrelease.Asset{
					Name:               name,
					BrowserDownloadURL: srv.URL + "/dl/" + name,
				})
			}
			_ = json.NewEncoder(w).Encode(rel)
		case strings.HasPrefix(r.URL.Path, "/dl/"):
			name := strings.TrimPrefix(r.URL.Path, "/dl/")
			if body, ok := assets[name]; ok {
				_, _ = w.Write(body)
				return
			}
			http.NotFound(w, r)
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

func TestCheckUpToDate(t *testing.T) {
	fakeRelease(t, "v1.0.0", nil)
	res, err := Check(Options{CurrentVersion: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Available {
		t.Errorf("expected not available when current == latest")
	}
}

func TestCheckNewerAvailable(t *testing.T) {
	fakeRelease(t, "v1.2.0", nil)
	res, err := Check(Options{CurrentVersion: "1.0.0"}) // unprefixed current
	if err != nil {
		t.Fatal(err)
	}
	if !res.Available {
		t.Errorf("expected available when latest > current")
	}
	if res.LatestVersion != "v1.2.0" {
		t.Errorf("LatestVersion = %q, want v1.2.0", res.LatestVersion)
	}
}

func TestCheckDevBuild(t *testing.T) {
	fakeRelease(t, "v1.0.0", nil)
	res, err := Check(Options{CurrentVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Available {
		t.Errorf("dev build should not be 'available' without --force")
	}

	res, err = Check(Options{CurrentVersion: "dev", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Available {
		t.Errorf("dev build with Force should be available")
	}
}

func TestRunNoOpWhenCurrent(t *testing.T) {
	fakeRelease(t, "v1.0.0", nil)
	res, err := Run(Options{CurrentVersion: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied {
		t.Errorf("Run should be a no-op when already current")
	}
}

func TestRunDryRunWritesNothing(t *testing.T) {
	asset := AssetName()
	fakeRelease(t, "v2.0.0", map[string][]byte{asset: []byte("new-binary")})

	exe := writeFakeBinary(t, "old-binary")
	res, err := Run(Options{CurrentVersion: "v1.0.0", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied {
		t.Errorf("dry-run should not apply")
	}
	if got := readFile(t, exe); got != "old-binary" {
		t.Errorf("binary changed in dry-run: %q", got)
	}
	if _, err := os.Stat(exe + ".dots-bak"); !os.IsNotExist(err) {
		t.Errorf("dry-run should not create a backup")
	}
}

func TestRunAppliesUpgrade(t *testing.T) {
	asset := AssetName()
	fakeRelease(t, "v2.0.0", map[string][]byte{asset: []byte("new-binary")})

	exe := writeFakeBinary(t, "old-binary")
	res, err := Run(Options{CurrentVersion: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied {
		t.Fatal("expected Applied")
	}
	if got := readFile(t, exe); got != "new-binary" {
		t.Errorf("binary = %q, want new-binary", got)
	}
	if got := readFile(t, res.BackupPath); got != "old-binary" {
		t.Errorf("backup = %q, want old-binary", got)
	}
	// Replaced binary must be executable.
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("upgraded binary not executable: %v", info.Mode())
	}
}

func TestRunMissingAsset(t *testing.T) {
	fakeRelease(t, "v2.0.0", map[string][]byte{"dots_other_platform": []byte("x")})
	writeFakeBinary(t, "old")
	if _, err := Run(Options{CurrentVersion: "v1.0.0"}); err == nil {
		t.Fatal("expected error when no matching asset")
	}
}

func TestRunPinnedVersion(t *testing.T) {
	asset := AssetName()
	fakeRelease(t, "v1.5.0", map[string][]byte{asset: []byte("pinned")})
	exe := writeFakeBinary(t, "old")
	res, err := Run(Options{CurrentVersion: "v1.0.0", TargetVersion: "1.5.0"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || readFile(t, exe) != "pinned" {
		t.Errorf("pinned upgrade did not apply: %+v", res)
	}
}

// writeFakeBinary creates a stand-in binary in a temp dir and points
// osExecutable at it for the test.
func writeFakeBinary(t *testing.T, content string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "dots")
	if err := os.WriteFile(exe, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	orig := osExecutable
	osExecutable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { osExecutable = orig })
	return exe
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
