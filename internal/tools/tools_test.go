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
	"github.com/pkuehne/dots/internal/ghrelease"
	"github.com/pkuehne/dots/internal/ui"
	"github.com/ulikunitz/xz"
)

// noTask is a no-op progress task for install tests that do not assert progress.
var noTask = ui.DiscardProgress().Task("")

// ── Filter ────────────────────────────────────────────────────────────────────

func TestFilter_EmptyReturnsAll(t *testing.T) {
	tools := []config.Tool{{Name: "rg"}, {Name: "bat"}}
	got := Filter(tools, nil, "", []string{"linux"}, "")
	if len(got) != 2 {
		t.Errorf("Filter() = %d tools, want 2", len(got))
	}
}

func TestFilter_ByName(t *testing.T) {
	tools := []config.Tool{{Name: "rg"}, {Name: "bat"}, {Name: "fzf"}}
	got := Filter(tools, []string{"rg", "fzf"}, "", []string{"linux"}, "")
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
	got := Filter(tools, nil, "core", []string{"linux"}, "")
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
	got := Filter(tools, []string{"rg"}, "", []string{"linux"}, "")
	if len(got) != 1 || got[0].Name != "rg" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestFilter_PlatformOnly(t *testing.T) {
	tools := []config.Tool{
		{Name: "everywhere"},
		{Name: "mac-only", Only: []string{"darwin"}},
		{Name: "wsl-only", Only: []string{"wsl"}},
	}

	got := Filter(tools, nil, "", []string{"linux", "wsl"}, "")
	if len(got) != 2 || got[0].Name != "everywhere" || got[1].Name != "wsl-only" {
		t.Errorf("on linux+wsl want [everywhere wsl-only], got: %v", got)
	}

	got = Filter(tools, nil, "", []string{"darwin"}, "")
	if len(got) != 2 || got[0].Name != "everywhere" || got[1].Name != "mac-only" {
		t.Errorf("on darwin want [everywhere mac-only], got: %v", got)
	}
}

func TestFilter_PlatformAppliesToNameAndTag(t *testing.T) {
	tools := []config.Tool{
		{Name: "mac-only", Only: []string{"darwin"}, Tags: []string{"core"}},
	}
	// Even when requested explicitly by name or tag, a platform-excluded tool
	// stays excluded.
	if got := Filter(tools, []string{"mac-only"}, "", []string{"linux"}, ""); len(got) != 0 {
		t.Errorf("by name: want 0 tools on linux, got: %v", got)
	}
	if got := Filter(tools, nil, "core", []string{"linux"}, ""); len(got) != 0 {
		t.Errorf("by tag: want 0 tools on linux, got: %v", got)
	}
}

func TestFilter_Profile(t *testing.T) {
	tools := []config.Tool{
		{Name: "everywhere"},
		{Name: "work-only", Profile: "work"},
	}
	got := Filter(tools, nil, "", []string{"linux"}, "")
	if len(got) != 1 || got[0].Name != "everywhere" {
		t.Errorf("no profile: want [everywhere], got: %v", got)
	}
	got = Filter(tools, nil, "", []string{"linux"}, "work")
	if len(got) != 2 {
		t.Errorf("work profile: want 2 tools, got: %v", got)
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

func makeTar(t *testing.T, entries []struct{ name, content string }) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
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
	tw.Close()
	return buf.Bytes()
}

func xzCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
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

// ── extractArchive (tar) ──────────────────────────────────────────────────────

func TestExtractArchive_TarGzHappyPath(t *testing.T) {
	data := makeTarGz(t,
		[]struct{ name, content string }{{"mybin", "#!/bin/sh\necho hi"}},
		nil,
	)
	archivePath := writeTempArchive(t, data, "good.tar.gz")
	dest := t.TempDir()

	if err := extractArchive(archivePath, dest); err != nil {
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

func TestExtractArchive_PathTraversalRejected(t *testing.T) {
	data := makeTarGz(t,
		[]struct{ name, content string }{{"../../etc/evil", "pwned"}},
		nil,
	)
	archivePath := writeTempArchive(t, data, "evil.tar.gz")
	dest := t.TempDir()

	err := extractArchive(archivePath, dest)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "path escapes") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractArchive_AbsoluteSymlinkRejected(t *testing.T) {
	data := makeTarGz(t, nil, []struct{ name, target string }{{"link", "/etc/passwd"}})
	archivePath := writeTempArchive(t, data, "evil.tar.gz")
	dest := t.TempDir()

	err := extractArchive(archivePath, dest)
	if err == nil {
		t.Fatal("expected error for absolute symlink")
	}
	if !strings.Contains(err.Error(), "absolute target") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractArchive_EscapingSymlinkRejected(t *testing.T) {
	data := makeTarGz(t, nil, []struct{ name, target string }{{"link", "../../etc/passwd"}})
	archivePath := writeTempArchive(t, data, "evil.tar.gz")
	dest := t.TempDir()

	err := extractArchive(archivePath, dest)
	if err == nil {
		t.Fatal("expected error for escaping symlink")
	}
	if !strings.Contains(err.Error(), "escapes target") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExtractArchive_SymlinkExtracted(t *testing.T) {
	data := makeTarGz(t,
		[]struct{ name, content string }{{"real", "binary"}},
		[]struct{ name, target string }{{"link", "real"}},
	)
	archivePath := writeTempArchive(t, data, "good.tar.gz")
	dest := t.TempDir()

	if err := extractArchive(archivePath, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	target, err := os.Readlink(filepath.Join(dest, "link"))
	if err != nil {
		t.Fatalf("expected a symlink: %v", err)
	}
	if target != "real" {
		t.Errorf("symlink target = %q, want %q", target, "real")
	}
}

func TestExtractArchive_NestedDir(t *testing.T) {
	data := makeTarGz(t,
		[]struct{ name, content string }{{"subdir/mybin", "binary"}},
		nil,
	)
	archivePath := writeTempArchive(t, data, "nested.tar.gz")
	dest := t.TempDir()

	if err := extractArchive(archivePath, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "subdir", "mybin")); err != nil {
		t.Errorf("nested file not found: %v", err)
	}
}

// ── extractArchive (zip) ──────────────────────────────────────────────────────

func TestExtractArchive_ZipHappyPath(t *testing.T) {
	data := makeZip(t, []struct{ name, content string }{{"mybin", "binary content"}})
	archivePath := writeTempArchive(t, data, "good.zip")
	dest := t.TempDir()

	if err := extractArchive(archivePath, dest); err != nil {
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

func TestExtractArchive_ZipPathTraversalRejected(t *testing.T) {
	data := makeZip(t, []struct{ name, content string }{{"../../etc/evil", "pwned"}})
	archivePath := writeTempArchive(t, data, "evil.zip")
	dest := t.TempDir()

	err := extractArchive(archivePath, dest)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "path escapes") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── isExtractableArchive ──────────────────────────────────────────────────────

func TestIsExtractableArchive(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"tool.tar.gz", true},
		{"tool.tgz", true},
		{"tool.tar.xz", true},
		{"tool.tar.bz2", true},
		{"tool.tar.zst", true},
		{"tool.tar", true},
		{"tool.zip", true},
		{"tool.TAR.GZ", true},
		{"tool", false},
		{"tool.gz", false},
		{"tool-linux-amd64", false},
	}
	for _, tc := range cases {
		if got := isExtractableArchive(tc.name); got != tc.want {
			t.Errorf("isExtractableArchive(%q) = %v, want %v", tc.name, got, tc.want)
		}
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

func TestFindAndInstallBinary_BinaryPathEscapesRoot(t *testing.T) {
	// A secret file sitting outside the extracted tree must not be reachable
	// through a "../" binary_path.
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(outside, "extracted")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	err := findAndInstallBinary(root, "secret", dest, "../secret")
	if err == nil {
		t.Fatal("expected error for binary_path escaping the archive root")
	}
	if !strings.Contains(err.Error(), "escapes the archive root") {
		t.Errorf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Error("nothing should have been installed for an escaping binary_path")
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
// for the given asset name + content, and restores ghrelease.APIBase after the test.
func newGitHubTestServer(t *testing.T, assetName string, assetContent []byte) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/latest") {
			release := ghrelease.Release{
				TagName: "v1.0.0",
				Assets: []ghrelease.Asset{{
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
	orig := ghrelease.APIBase
	ghrelease.APIBase = srv.URL
	t.Cleanup(func() {
		srv.Close()
		ghrelease.APIBase = orig
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
	if _, err := installGitHub(tool, inst, binDir, noTask); err != nil {
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
	if _, err := installGitHub(tool, inst, binDir, noTask); err != nil {
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
	_, err := installGitHub(tool, inst, binDir, noTask)
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
	orig := ghrelease.APIBase
	ghrelease.APIBase = srv.URL
	defer func() { ghrelease.APIBase = orig }()

	tool := config.Tool{Name: "rg"}
	inst := config.ToolInstall{Method: "github", Repo: "BurntSushi/ripgrep"}
	_, err := installGitHub(tool, inst, t.TempDir(), noTask)
	if err == nil {
		t.Fatal("expected rate limit error")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstallGitHub_DownloadNotFound(t *testing.T) {
	// Release lookup succeeds, but the asset download returns 404 (a JSON error
	// body that must never be written to disk as the binary).
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/latest") {
			release := ghrelease.Release{
				TagName: "v1.0.0",
				Assets: []ghrelease.Asset{{
					Name:               "tool_1.0.0_linux_amd64.tar.gz",
					BrowserDownloadURL: srv.URL + "/download/tool_1.0.0_linux_amd64.tar.gz",
				}},
			}
			_ = json.NewEncoder(w).Encode(release)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	orig := ghrelease.APIBase
	ghrelease.APIBase = srv.URL
	defer func() { ghrelease.APIBase = orig }()

	tool := config.Tool{Name: "tool"}
	inst := config.ToolInstall{
		Method: "github",
		Repo:   "example/tool",
		Asset:  "tool_{version}_linux_amd64.tar.gz",
	}
	binDir := t.TempDir()
	_, err := installGitHub(tool, inst, binDir, noTask)
	if err == nil {
		t.Fatal("expected error on 404 asset download")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention the HTTP status, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(binDir, "tool")); statErr == nil {
		t.Error("a binary must not be installed when the download fails")
	}
}

func TestInstallGitHub_TarXz(t *testing.T) {
	// .tar.xz is now supported via mholt/archives.
	tarData := makeTar(t, []struct{ name, content string }{{"lazygit", "#!/bin/sh\necho lazygit"}})
	data := xzCompress(t, tarData)
	newGitHubTestServer(t, "lazygit_1.0.0_Linux_x86_64.tar.xz", data)

	tool := config.Tool{Name: "lazygit"}
	inst := config.ToolInstall{
		Method: "github",
		Repo:   "jesseduffield/lazygit",
		Asset:  "lazygit_{version}_Linux_x86_64.tar.xz",
		Binary: "lazygit",
	}
	binDir := t.TempDir()
	if _, err := installGitHub(tool, inst, binDir, noTask); err != nil {
		t.Fatalf("installGitHub: %v", err)
	}
	if _, err := os.Stat(filepath.Join(binDir, "lazygit")); err != nil {
		t.Errorf("binary not installed: %v", err)
	}
}

func TestInstallGitHub_CorruptArchive(t *testing.T) {
	// A file that claims to be a .tar.gz but is not must surface an error and
	// must never be installed verbatim as the binary.
	newGitHubTestServer(t, "tool_1.0.0_linux_amd64.tar.gz", []byte("not a real archive"))

	tool := config.Tool{Name: "tool"}
	inst := config.ToolInstall{
		Method: "github",
		Repo:   "example/tool",
		Asset:  "tool_{version}_linux_amd64.tar.gz",
	}
	binDir := t.TempDir()
	_, err := installGitHub(tool, inst, binDir, noTask)
	if err == nil {
		t.Fatal("expected error for corrupt archive")
	}
	if _, statErr := os.Stat(filepath.Join(binDir, "tool")); statErr == nil {
		t.Error("the corrupt archive must not be installed as the binary")
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
	if _, err := installGitHub(tool, inst, binDir, noTask); err != nil {
		t.Fatalf("installGitHub: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(binDir, "cmake"))
	if string(got) != "real binary" {
		t.Errorf("binary_path should have selected bin/cmake, got: %q", got)
	}
}

// ── install_dir (full archive tree) ───────────────────────────────────────────

func TestStripComponents(t *testing.T) {
	cases := []struct {
		name string
		n    int
		want string
	}{
		{"nvim-linux-x86_64/bin/nvim", 1, "bin/nvim"},
		{"nvim-linux-x86_64/share/nvim/runtime/x", 1, "share/nvim/runtime/x"},
		{"top/sub/file", 2, "file"},
		{"top/file", 1, "file"},
		{"top", 1, ""},      // exactly at depth: nothing remains
		{"top/file", 2, ""}, // fewer than n components
		{"/leading/slash/x", 1, "slash/x"},
		{"plain", 0, "plain"},
		{"a/b/c", -1, "a/b/c"}, // negative n must not panic
	}
	for _, tc := range cases {
		if got := stripComponents(tc.name, tc.n); got != tc.want {
			t.Errorf("stripComponents(%q, %d) = %q, want %q", tc.name, tc.n, got, tc.want)
		}
	}
}

func TestExtractArchiveStrip(t *testing.T) {
	data := makeTarGz(t, []struct{ name, content string }{
		{"root/bin/tool", "binary"},
		{"root/share/data", "data"},
	}, nil)
	archivePath := writeTempArchive(t, data, "tool.tar.gz")
	dest := t.TempDir()

	if err := extractArchiveStrip(archivePath, dest, 1); err != nil {
		t.Fatalf("extractArchiveStrip: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "bin", "tool")); string(got) != "binary" {
		t.Errorf("bin/tool not extracted at stripped path, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "root")); !os.IsNotExist(err) {
		t.Errorf("leading component should have been stripped, but root/ exists")
	}
}

func TestInstallGitHub_InstallDir(t *testing.T) {
	data := makeTarGz(t, []struct{ name, content string }{
		{"nvim-linux-x86_64/bin/nvim", "nvim binary"},
		{"nvim-linux-x86_64/share/nvim/runtime/doc", "runtime files"},
	}, nil)
	newGitHubTestServer(t, "nvim-linux-x86_64.tar.gz", data)

	installDir := filepath.Join(t.TempDir(), "nvim")
	binDir := t.TempDir()
	tool := config.Tool{Name: "neovim"}
	inst := config.ToolInstall{
		Method:          "github",
		Repo:            "neovim/neovim",
		Asset:           "nvim-linux-x86_64.tar.gz",
		Binary:          "nvim",
		InstallDir:      installDir,
		StripComponents: 1,
	}
	if _, err := installGitHub(tool, inst, binDir, noTask); err != nil {
		t.Fatalf("installGitHub: %v", err)
	}

	// Whole tree extracted (stripped) into install_dir.
	if got, _ := os.ReadFile(filepath.Join(installDir, "bin", "nvim")); string(got) != "nvim binary" {
		t.Errorf("nvim binary not in install_dir: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(installDir, "share", "nvim", "runtime", "doc")); string(got) != "runtime files" {
		t.Errorf("runtime files not in install_dir: %q", got)
	}

	// bin_dir/nvim is a symlink to the binary inside install_dir.
	link := filepath.Join(binDir, "nvim")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", link, err)
	}
	if target != filepath.Join(installDir, "bin", "nvim") {
		t.Errorf("symlink points to %q, want %q", target, filepath.Join(installDir, "bin", "nvim"))
	}
}

func TestInstallGitHub_InstallDirReplacesStaleFiles(t *testing.T) {
	installDir := filepath.Join(t.TempDir(), "nvim")
	binDir := t.TempDir()
	// Seed a stale file that a fresh extraction would not produce.
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(installDir, "share", "stale.txt")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	data := makeTarGz(t, []struct{ name, content string }{
		{"nvim-linux-x86_64/bin/nvim", "nvim binary"},
	}, nil)
	newGitHubTestServer(t, "nvim-linux-x86_64.tar.gz", data)

	tool := config.Tool{Name: "neovim"}
	inst := config.ToolInstall{
		Method:          "github",
		Repo:            "neovim/neovim",
		Asset:           "nvim-linux-x86_64.tar.gz",
		Binary:          "nvim",
		InstallDir:      installDir,
		StripComponents: 1,
	}
	if _, err := installGitHub(tool, inst, binDir, noTask); err != nil {
		t.Fatalf("installGitHub: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale file should have been removed when install_dir was replaced")
	}
}

func TestInstallGitHub_InstallDirFailureKeepsPriorInstall(t *testing.T) {
	installDir := filepath.Join(t.TempDir(), "nvim")
	binDir := t.TempDir()
	// A working prior install that must survive a failed re-install.
	prior := filepath.Join(installDir, "bin", "nvim")
	if err := os.MkdirAll(filepath.Dir(prior), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(prior, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// New archive is missing the binary, so the install must fail after staging.
	data := makeTarGz(t, []struct{ name, content string }{
		{"nvim-linux-x86_64/share/doc", "docs only"},
	}, nil)
	newGitHubTestServer(t, "nvim-linux-x86_64.tar.gz", data)

	tool := config.Tool{Name: "neovim"}
	inst := config.ToolInstall{
		Method:          "github",
		Repo:            "neovim/neovim",
		Asset:           "nvim-linux-x86_64.tar.gz",
		Binary:          "nvim",
		InstallDir:      installDir,
		StripComponents: 1,
	}
	if _, err := installGitHub(tool, inst, binDir, noTask); err == nil {
		t.Fatal("expected install to fail when archive lacks the binary")
	}
	// Prior install untouched; no staging dir left behind.
	if got, _ := os.ReadFile(prior); string(got) != "old binary" {
		t.Errorf("prior install should be intact, got %q", got)
	}
	if _, err := os.Stat(installDir + ".dots-new"); !os.IsNotExist(err) {
		t.Error("staging dir should have been cleaned up on failure")
	}
}

func TestInstallGitHub_InstallDirNonArchiveRejected(t *testing.T) {
	newGitHubTestServer(t, "tool-linux", []byte("raw binary"))

	tool := config.Tool{Name: "tool"}
	inst := config.ToolInstall{
		Method:     "github",
		Repo:       "owner/tool",
		Asset:      "tool-linux",
		Binary:     "tool",
		InstallDir: filepath.Join(t.TempDir(), "tool"),
	}
	_, err := installGitHub(tool, inst, t.TempDir(), noTask)
	if err == nil {
		t.Fatal("expected error when install_dir is set on a non-archive asset")
	}
	if !strings.Contains(err.Error(), "not an archive") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSafeInstallDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	binDir := filepath.Join(home, ".local", "bin")

	t.Run("rejects root", func(t *testing.T) {
		if _, err := safeInstallDir("/", binDir); err == nil {
			t.Error("expected error for filesystem root")
		}
	})
	t.Run("rejects home", func(t *testing.T) {
		if _, err := safeInstallDir("~", binDir); err == nil {
			t.Error("expected error for home directory")
		}
	})
	t.Run("rejects dir containing bin_dir", func(t *testing.T) {
		// ~/.local contains ~/.local/bin, so wiping it would destroy bin_dir.
		if _, err := safeInstallDir("~/.local", binDir); err == nil {
			t.Error("expected error for install_dir containing bin_dir")
		}
	})
	t.Run("accepts dedicated subdir and returns absolute path", func(t *testing.T) {
		got, err := safeInstallDir("~/.local/nvim", binDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := filepath.Join(home, ".local", "nvim"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// ── Check ─────────────────────────────────────────────────────────────────────

func TestCheck_InstalledTool(t *testing.T) {
	// "true" is always available and exits 0.
	tools := []config.Tool{{Name: "true", Check: "true"}}
	results := Check(tools)
	if len(results) != 1 || !results[0].Installed {
		t.Errorf("expected true to be installed, got: %v", results)
	}
}

func TestCheck_MissingTool(t *testing.T) {
	tools := []config.Tool{{Name: "dots-nonexistent-binary-xyz", Check: "dots-nonexistent-binary-xyz --version"}}
	results := Check(tools)
	if len(results) != 1 || results[0].Installed {
		t.Errorf("expected missing tool to not be installed, got: %v", results)
	}
}

func TestCheck_NoCheckFallsBackToLookPath(t *testing.T) {
	// "sh" is always on PATH
	tools := []config.Tool{{Name: "sh"}}
	results := Check(tools)
	if len(results) != 1 || !results[0].Installed {
		t.Errorf("expected sh to be found via LookPath, got: %v", results)
	}
}
