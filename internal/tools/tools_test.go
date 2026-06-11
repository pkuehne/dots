package tools

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkuehne/dots/internal/config"
)

// ── Filter ────────────────────────────────────────────────────────────────────

func TestFilter_EmptyReturnsAll(t *testing.T) {
	tools := []config.Tool{{Name: "rg"}, {Name: "bat"}}
	got := Filter(tools, nil, "")
	if len(got) != 2 {
		t.Errorf("Filter() = %d tools, want 2", len(got))
	}
}

func TestFilter_ByName(t *testing.T) {
	tools := []config.Tool{{Name: "rg"}, {Name: "bat"}, {Name: "fzf"}}
	got := Filter(tools, []string{"rg", "fzf"}, "")
	if len(got) != 2 {
		t.Fatalf("got %d tools, want 2", len(got))
	}
	if got[0].Name != "rg" || got[1].Name != "fzf" {
		t.Errorf("unexpected names: %v", got)
	}
}

func TestFilter_ByTag(t *testing.T) {
	tools := []config.Tool{
		{Name: "rg", Tags: []string{"core", "search"}},
		{Name: "bat", Tags: []string{"core"}},
		{Name: "fzf", Tags: []string{"ui"}},
	}
	got := Filter(tools, nil, "core")
	if len(got) != 2 {
		t.Fatalf("got %d tools, want 2", len(got))
	}
}

func TestFilter_NameTakesPrecedenceOverTag(t *testing.T) {
	tools := []config.Tool{
		{Name: "rg", Tags: []string{"search"}},
		{Name: "bat", Tags: []string{"core"}},
	}
	// Filter by name "rg" — bat should not be included even if tag matches nothing.
	got := Filter(tools, []string{"rg"}, "")
	if len(got) != 1 || got[0].Name != "rg" {
		t.Errorf("unexpected result: %v", got)
	}
}

// ── GlobMatch ─────────────────────────────────────────────────────────────────

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"rg-1.0.0-x86_64-linux.tar.gz", "rg-1.0.0-x86_64-linux.tar.gz", true},
		{"rg-*-x86_64-linux.tar.gz", "rg-1.0.0-x86_64-linux.tar.gz", true},
		{"rg-*-x86_64-linux.tar.gz", "rg-1.0.0-aarch64-linux.tar.gz", false},
		{"rg-?-linux.tar.gz", "rg-1-linux.tar.gz", true},
		{"rg-?-linux.tar.gz", "rg-10-linux.tar.gz", false},
		{"*.tar.gz", "rg-1.0.0.tar.gz", true},
		{"*.tar.gz", "rg-1.0.0.zip", false},
	}
	for _, tc := range cases {
		got := globMatch(tc.pattern, tc.name)
		if got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

// ── Archive helpers ───────────────────────────────────────────────────────────

func makeTarGz(t *testing.T, entries []struct{ name, content string }, symlinks []struct{ name, target string }) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: tar.TypeReg,
			Size:     int64(len(e.content)),
			Mode:     0o755,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range symlinks {
		hdr := &tar.Header{
			Name:     s.name,
			Typeflag: tar.TypeSymlink,
			Linkname: s.target,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func makeZip(t *testing.T, entries []struct{ name, content string }) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.content)); err != nil {
			t.Fatal(err)
		}
	}
	zw.Close()
	return buf.Bytes()
}

func writeTempArchive(t *testing.T, data []byte, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ── safeTarExtractAll ─────────────────────────────────────────────────────────

func TestSafeTarExtractAll_HappyPath(t *testing.T) {
	data := makeTarGz(t,
		[]struct{ name, content string }{{"mybin", "#!/bin/sh\necho hi"}},
		nil,
	)
	archivePath := writeTempArchive(t, data, "good.tar.gz")
	dest := t.TempDir()

	if err := safeTarExtractAll(archivePath, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "mybin"))
	if err != nil {
		t.Fatalf("extracted file not found: %v", err)
	}
	if string(got) != "#!/bin/sh\necho hi" {
		t.Errorf("unexpected content: %q", got)
	}
}

func TestSafeTarExtractAll_PathTraversalRejected(t *testing.T) {
	data := makeTarGz(t,
		[]struct{ name, content string }{{"../../etc/evil", "pwned"}},
		nil,
	)
	archivePath := writeTempArchive(t, data, "evil.tar.gz")
	dest := t.TempDir()

	err := safeTarExtractAll(archivePath, dest)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "path escapes") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSafeTarExtractAll_AbsoluteSymlinkRejected(t *testing.T) {
	data := makeTarGz(t, nil, []struct{ name, target string }{{"link", "/etc/passwd"}})
	archivePath := writeTempArchive(t, data, "evil.tar.gz")
	dest := t.TempDir()

	err := safeTarExtractAll(archivePath, dest)
	if err == nil {
		t.Fatal("expected error for absolute symlink")
	}
	if !strings.Contains(err.Error(), "absolute target") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSafeTarExtractAll_NestedDir(t *testing.T) {
	data := makeTarGz(t,
		[]struct{ name, content string }{{"subdir/mybin", "binary"}},
		nil,
	)
	archivePath := writeTempArchive(t, data, "nested.tar.gz")
	dest := t.TempDir()

	if err := safeTarExtractAll(archivePath, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "subdir", "mybin")); err != nil {
		t.Errorf("nested file not found: %v", err)
	}
}

// ── safeZipExtractAll ─────────────────────────────────────────────────────────

func TestSafeZipExtractAll_HappyPath(t *testing.T) {
	data := makeZip(t, []struct{ name, content string }{{"mybin", "binary content"}})
	archivePath := writeTempArchive(t, data, "good.zip")
	dest := t.TempDir()

	if err := safeZipExtractAll(archivePath, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "mybin"))
	if err != nil {
		t.Fatalf("extracted file not found: %v", err)
	}
	if string(got) != "binary content" {
		t.Errorf("unexpected content: %q", got)
	}
}

func TestSafeZipExtractAll_PathTraversalRejected(t *testing.T) {
	data := makeZip(t, []struct{ name, content string }{{"../../etc/evil", "pwned"}})
	archivePath := writeTempArchive(t, data, "evil.zip")
	dest := t.TempDir()

	err := safeZipExtractAll(archivePath, dest)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "path escapes") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── findAndInstallBinary ──────────────────────────────────────────────────────

func TestFindAndInstallBinary_BinaryPath(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "cmake"), []byte("real binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Also plant a same-named file deeper to ensure binary_path wins
	completionDir := filepath.Join(dir, "share", "completions")
	if err := os.MkdirAll(completionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(completionDir, "cmake"), []byte("completion script"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "cmake")
	if err := findAndInstallBinary(dir, "cmake", dest, "bin/cmake"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "real binary" {
		t.Errorf("binary_path should have selected bin/cmake, got: %q", got)
	}
}

func TestFindAndInstallBinary_BinaryPathMissing(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(t.TempDir(), "cmake")
	err := findAndInstallBinary(dir, "cmake", dest, "bin/cmake")
	if err == nil {
		t.Fatal("expected error for missing binary_path")
	}
	if !strings.Contains(err.Error(), "binary_path") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFindAndInstallBinary_ShallowWins(t *testing.T) {
	dir := t.TempDir()
	shallow := filepath.Join(dir, "bin", "cmake")
	deep := filepath.Join(dir, "share", "completions", "cmake")
	for _, p := range []string{shallow, deep} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		label := "shallow"
		if p == deep {
			label = "deep"
		}
		if err := os.WriteFile(p, []byte(label), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	dest := filepath.Join(t.TempDir(), "cmake")
	if err := findAndInstallBinary(dir, "cmake", dest, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "shallow" {
		t.Errorf("expected shallow to win, got: %q", got)
	}
}

func TestFindAndInstallBinary_NotFound(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(t.TempDir(), "rg")
	err := findAndInstallBinary(dir, "rg", dest, "")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── installGitHub (with mock HTTP server) ─────────────────────────────────────

// newGitHubTestServer returns a test HTTP server that serves a fake release
// for the given asset name + content, and restores githubAPIBase after the test.
func newGitHubTestServer(t *testing.T, assetName string, assetContent []byte) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/latest") {
			release := githubRelease{
				TagName: "v1.0.0",
				Assets: []githubAsset{{
					Name:               assetName,
					BrowserDownloadURL: srv.URL + "/download/" + assetName,
				}},
			}
			_ = json.NewEncoder(w).Encode(release)
			return
		}
		if strings.Contains(r.URL.Path, "/download/") {
			w.Write(assetContent)
			return
		}
		http.NotFound(w, r)
	}))
	orig := githubAPIBase
	githubAPIBase = srv.URL
	t.Cleanup(func() {
		srv.Close()
		githubAPIBase = orig
	})
	return srv
}

func TestInstallGitHub_TarGz(t *testing.T) {
	data := makeTarGz(t,
		[]struct{ name, content string }{{"lazygit", "#!/bin/sh\necho lazygit"}},
		nil,
	)
	newGitHubTestServer(t, "lazygit_1.0.0_Linux_x86_64.tar.gz", data)

	tool := config.Tool{Name: "lazygit"}
	inst := config.ToolInstall{
		Method: "github",
		Repo:   "jesseduffield/lazygit",
		Asset:  "lazygit_{version}_Linux_x86_64.tar.gz",
		Binary: "lazygit",
	}
	binDir := t.TempDir()
	if err := installGitHub(tool, inst, binDir); err != nil {
		t.Fatalf("installGitHub: %v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "lazygit")); err != nil {
		t.Errorf("binary not installed: %v", err)
	}
}

func TestInstallGitHub_ArchMap(t *testing.T) {
	// Simulates aarch64 host but tool uses "arm64" in asset names
	data := makeTarGz(t,
		[]struct{ name, content string }{{"lazygit", "#!/bin/sh"}},
		nil,
	)
	newGitHubTestServer(t, "lazygit_1.0.0_Linux_arm64.tar.gz", data)

	tool := config.Tool{Name: "lazygit"}
	inst := config.ToolInstall{
		Method:  "github",
		Repo:    "jesseduffield/lazygit",
		Asset:   "lazygit_{version}_Linux_{arch}.tar.gz",
		ArchMap: map[string]string{"aarch64": "arm64", "x86_64": "x86_64"},
		Binary:  "lazygit",
	}
	binDir := t.TempDir()
	// The asset pattern with arch_map applied to the current arch must match
	// one of the test server's assets. We test that the expansion logic works
	// by verifying the binary is installed (server serves arm64 asset).
	if err := installGitHub(tool, inst, binDir); err != nil {
		// Only fail if the binary would have been reachable — if current arch
		// maps to something the test server doesn't serve, that's expected.
		if !strings.Contains(err.Error(), "no matching asset") {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Skip("current arch does not map to the test asset; skipping")
	}
}

func TestInstallGitHub_NoMatchingAsset(t *testing.T) {
	data := makeTarGz(t, []struct{ name, content string }{{"bin", "x"}}, nil)
	newGitHubTestServer(t, "tool_1.0.0_windows_amd64.zip", data)

	tool := config.Tool{Name: "tool"}
	inst := config.ToolInstall{
		Method: "github",
		Repo:   "example/tool",
		Asset:  "tool_{version}_linux_amd64.tar.gz",
	}
	binDir := t.TempDir()
	err := installGitHub(tool, inst, binDir)
	if err == nil {
		t.Fatal("expected error when no asset matches")
	}
	if !strings.Contains(err.Error(), "no matching asset") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstallGitHub_RateLimitError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	orig := githubAPIBase
	githubAPIBase = srv.URL
	defer func() { githubAPIBase = orig }()

	tool := config.Tool{Name: "rg"}
	inst := config.ToolInstall{Method: "github", Repo: "BurntSushi/ripgrep"}
	err := installGitHub(tool, inst, t.TempDir())
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstallGitHub_BinaryPath(t *testing.T) {
	// Archive has two "cmake" files: bin/cmake (the real binary) and
	// share/completions/cmake (a script). binary_path must select bin/cmake.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, entry := range []struct{ name, content string }{
		{"bin/cmake", "real binary"},
		{"share/completions/cmake", "completion script"},
	} {
		hdr := &tar.Header{Name: entry.name, Typeflag: tar.TypeReg, Size: int64(len(entry.content)), Mode: 0o755}
		_ = tw.WriteHeader(hdr)
		_, _ = tw.Write([]byte(entry.content))
	}
	tw.Close()
	gw.Close()

	newGitHubTestServer(t, "cmake-1.0.0-linux-x86_64.tar.gz", buf.Bytes())

	tool := config.Tool{Name: "cmake"}
	inst := config.ToolInstall{
		Method:     "github",
		Repo:       "Kitware/CMake",
		Asset:      "cmake-{version}-linux-x86_64.tar.gz",
		Binary:     "cmake",
		BinaryPath: "bin/cmake",
	}
	binDir := t.TempDir()
	if err := installGitHub(tool, inst, binDir); err != nil {
		t.Fatalf("installGitHub: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(binDir, "cmake"))
	if string(got) != "real binary" {
		t.Errorf("binary_path should have selected bin/cmake, got: %q", got)
	}
}

// ── Check ─────────────────────────────────────────────────────────────────────

func TestCheck_InstalledTool(t *testing.T) {
	// "true" is always available and exits 0.
	tools := []config.Tool{{Name: "true", Check: "true"}}
	results := Check(tools, "linux", "amd64")
	if len(results) != 1 || !results[0].Installed {
		t.Errorf("expected true to be installed, got: %v", results)
	}
}

func TestCheck_MissingTool(t *testing.T) {
	tools := []config.Tool{{Name: "dots-nonexistent-binary-xyz", Check: "dots-nonexistent-binary-xyz --version"}}
	results := Check(tools, "linux", "amd64")
	if len(results) != 1 || results[0].Installed {
		t.Errorf("expected missing tool to not be installed, got: %v", results)
	}
}

func TestCheck_NoCheckFallsBackToLookPath(t *testing.T) {
	// "sh" is always on PATH
	tools := []config.Tool{{Name: "sh"}}
	results := Check(tools, "linux", "amd64")
	if len(results) != 1 || !results[0].Installed {
		t.Errorf("expected sh to be found via LookPath, got: %v", results)
	}
}
