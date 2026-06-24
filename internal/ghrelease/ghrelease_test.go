package ghrelease

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestServer serves a fake "latest" + "by tag" release and an asset, and
// points APIBase at itself for the duration of the test.
func newTestServer(t *testing.T, tag, assetName string, assetContent []byte) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/latest"),
			strings.Contains(r.URL.Path, "/releases/tags/"):
			_ = json.NewEncoder(w).Encode(Release{
				TagName: tag,
				Assets:  []Asset{{Name: assetName, BrowserDownloadURL: srv.URL + "/dl/" + assetName}},
			})
		case strings.Contains(r.URL.Path, "/dl/"):
			_, _ = w.Write(assetContent)
		default:
			http.NotFound(w, r)
		}
	}))
	orig := APIBase
	APIBase = srv.URL
	t.Cleanup(func() {
		srv.Close()
		APIBase = orig
	})
	return srv
}

func TestGetLatestRelease(t *testing.T) {
	newTestServer(t, "v1.2.3", "tool_linux_amd64", []byte("x"))
	rel, err := GetLatestRelease("owner/repo")
	if err != nil {
		t.Fatalf("GetLatestRelease: %v", err)
	}
	if rel.TagName != "v1.2.3" {
		t.Errorf("TagName = %q, want v1.2.3", rel.TagName)
	}
	if len(rel.Assets) != 1 || rel.Assets[0].Name != "tool_linux_amd64" {
		t.Errorf("unexpected assets: %+v", rel.Assets)
	}
}

func TestGetReleaseByTag(t *testing.T) {
	newTestServer(t, "v2.0.0", "a", nil)
	rel, err := GetReleaseByTag("owner/repo", "2.0.0")
	if err != nil {
		t.Fatalf("GetReleaseByTag: %v", err)
	}
	if rel.TagName != "v2.0.0" {
		t.Errorf("TagName = %q, want v2.0.0", rel.TagName)
	}
}

func TestGetReleaseByTagVPrefixFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/tags/v1.2.3") {
			_ = json.NewEncoder(w).Encode(Release{TagName: "v1.2.3"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	orig := APIBase
	APIBase = srv.URL
	defer func() { APIBase = orig }()

	// Bare version falls back to the v-prefixed tag.
	rel, err := GetReleaseByTag("user/repo", "1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel.TagName != "v1.2.3" {
		t.Errorf("TagName = %q, want v1.2.3", rel.TagName)
	}

	// Exact tag works directly.
	if rel, err := GetReleaseByTag("user/repo", "v1.2.3"); err != nil || rel.TagName != "v1.2.3" {
		t.Errorf("exact tag lookup failed: %v / %+v", err, rel)
	}

	// Missing tag surfaces an error.
	if _, err := GetReleaseByTag("user/repo", "9.9.9"); err == nil {
		t.Error("expected error for missing tag")
	}
}

func TestGetReleaseNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	orig := APIBase
	APIBase = srv.URL
	defer func() { APIBase = orig }()

	if _, err := GetLatestRelease("owner/repo"); err == nil {
		t.Fatal("expected error for 404 release")
	}
}

func TestDownloadAsset(t *testing.T) {
	content := []byte("binary-bytes")
	srv := newTestServer(t, "v1.0.0", "tool", content)
	rel, err := GetLatestRelease("owner/repo")
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "out")
	if err := DownloadAsset(rel.Assets[0].BrowserDownloadURL, dest); err != nil {
		t.Fatalf("DownloadAsset: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Errorf("downloaded %q, want %q", got, content)
	}
	_ = srv
}

func TestDownloadAssetHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if err := DownloadAsset(srv.URL+"/missing", filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("expected error for non-200 download")
	}
}
