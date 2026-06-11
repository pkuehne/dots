package repos

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkuehne/dots/internal/config"
)

// makeBarerepo initialises a bare git repo and returns its path.
func makeBareRepo(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "bare.git")
	if err := exec.Command("git", "init", "--bare", bare).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	return bare
}

// makeLocalRepo creates a non-bare repo with an initial commit and pushes to bare.
func makeLocalRepo(t *testing.T, bare string) string {
	t.Helper()
	local := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = local
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", args, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	run("git", "commit", "--allow-empty", "-m", "init")
	run("git", "remote", "add", "origin", bare)
	run("git", "push", "-u", "origin", "HEAD")
	return local
}

// ---------- Filter ----------

func TestFilter_Empty(t *testing.T) {
	repos := []config.RepoEntry{{Name: "a"}, {Name: "b"}}
	got := Filter(repos, nil, []string{"linux"}, "")
	if len(got) != 2 {
		t.Fatalf("want 2 repos, got %d", len(got))
	}
}

func TestFilter_ByName(t *testing.T) {
	repos := []config.RepoEntry{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	got := Filter(repos, []string{"a", "c"}, []string{"linux"}, "")
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if got[0].Name != "a" || got[1].Name != "c" {
		t.Fatalf("unexpected names: %v", got)
	}
}

func TestFilter_NoMatch(t *testing.T) {
	repos := []config.RepoEntry{{Name: "a"}}
	got := Filter(repos, []string{"z"}, []string{"linux"}, "")
	if len(got) != 0 {
		t.Fatalf("want 0, got %d", len(got))
	}
}

func TestFilter_PlatformOnly(t *testing.T) {
	repos := []config.RepoEntry{
		{Name: "everywhere"},
		{Name: "mac-only", Only: []string{"darwin"}},
	}
	got := Filter(repos, nil, []string{"linux"}, "")
	if len(got) != 1 || got[0].Name != "everywhere" {
		t.Fatalf("on linux want [everywhere], got: %v", got)
	}
	got = Filter(repos, nil, []string{"darwin"}, "")
	if len(got) != 2 {
		t.Fatalf("on darwin want 2 repos, got: %v", got)
	}
	// Even when requested explicitly by name, a platform-excluded repo stays excluded.
	got = Filter(repos, []string{"mac-only"}, []string{"linux"}, "")
	if len(got) != 0 {
		t.Fatalf("by name on linux want 0 repos, got: %v", got)
	}
}

func TestFilter_Profile(t *testing.T) {
	repos := []config.RepoEntry{
		{Name: "everywhere"},
		{Name: "work-only", Profile: "work"},
	}
	got := Filter(repos, nil, []string{"linux"}, "")
	if len(got) != 1 || got[0].Name != "everywhere" {
		t.Fatalf("no profile: want [everywhere], got: %v", got)
	}
	got = Filter(repos, nil, []string{"linux"}, "work")
	if len(got) != 2 {
		t.Fatalf("work profile: want 2 repos, got: %v", got)
	}
}

// ---------- expandURL ----------

func TestExpandURL_Shorthand(t *testing.T) {
	got := expandURL("user/repo")
	want := "https://github.com/user/repo"
	if got != want {
		t.Fatalf("want %q got %q", want, got)
	}
}

func TestExpandURL_FullHTTPS(t *testing.T) {
	url := "https://github.com/user/repo.git"
	if expandURL(url) != url {
		t.Fatal("full URL should be unchanged")
	}
}

func TestExpandURL_SSH(t *testing.T) {
	url := "git@github.com:user/repo.git"
	if expandURL(url) != url {
		t.Fatal("SSH URL should be unchanged")
	}
}

// ---------- cloneOne ----------

func TestCloneOne_AlreadyExists(t *testing.T) {
	dst := t.TempDir()
	// make it look like a git repo
	if err := os.Mkdir(filepath.Join(dst, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := config.RepoEntry{Name: "test", Repo: "user/test", Dst: dst}
	result, err := cloneOne(r, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "already" {
		t.Fatalf("want 'already', got %q", result)
	}
}

func TestCloneOne_DirExistsNotGit(t *testing.T) {
	dst := t.TempDir()
	r := config.RepoEntry{Name: "test", Repo: "user/test", Dst: dst}
	_, err := cloneOne(r, false)
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
	if !strings.Contains(err.Error(), "not a git repository") && !strings.Contains(err.Error(), "Cannot clone") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCloneOne_DryRun(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "new-repo")
	r := config.RepoEntry{Name: "test", Repo: "user/test", Dst: dst}
	result, err := cloneOne(r, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("want 'ok', got %q", result)
	}
	// dst should not have been created
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("dry-run must not create directory")
	}
}

func TestCloneOne_RealClone(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	bare := makeBareRepo(t)
	makeLocalRepo(t, bare) // seed the bare repo

	dst := filepath.Join(t.TempDir(), "clone")
	r := config.RepoEntry{Name: "test", Repo: "file://" + bare, Dst: dst}
	result, err := cloneOne(r, false)
	if err != nil {
		t.Fatalf("clone failed: %v", err)
	}
	if result != "ok" {
		t.Fatalf("want 'ok', got %q", result)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); err != nil {
		t.Fatal(".git dir not present after clone")
	}
}

func TestCloneOne_ShallowFlag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	bare := makeBareRepo(t)
	makeLocalRepo(t, bare)

	dst := filepath.Join(t.TempDir(), "shallow-clone")
	r := config.RepoEntry{Name: "test", Repo: "file://" + bare, Dst: dst, Shallow: true}
	if _, err := cloneOne(r, false); err != nil {
		t.Fatalf("shallow clone failed: %v", err)
	}
}

func TestCloneOne_OnInstallHook(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	bare := makeBareRepo(t)
	makeLocalRepo(t, bare)

	dst := filepath.Join(t.TempDir(), "hook-clone")
	marker := filepath.Join(dst, "hook_ran")
	r := config.RepoEntry{
		Name:      "test",
		Repo:      "file://" + bare,
		Dst:       dst,
		OnInstall: "touch hook_ran",
	}
	if _, err := cloneOne(r, false); err != nil {
		t.Fatalf("clone with hook failed: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("on_install hook did not run")
	}
}

// ---------- updateOne ----------

func TestUpdateOne_Missing(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "nope")
	r := config.RepoEntry{Name: "test", Repo: "user/test", Dst: dst}
	result, err := updateOne(r, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "missing" {
		t.Fatalf("want 'missing', got %q", result)
	}
}

func TestUpdateOne_DryRun(t *testing.T) {
	dst := t.TempDir()
	r := config.RepoEntry{Name: "test", Repo: "user/test", Dst: dst}
	result, err := updateOne(r, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("want 'ok', got %q", result)
	}
}

func TestUpdateOne_Pull(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	bare := makeBareRepo(t)
	local := makeLocalRepo(t, bare)

	// clone it so we have a proper remote-tracking branch
	dst := filepath.Join(t.TempDir(), "clone")
	r := config.RepoEntry{Name: "test", Repo: "file://" + bare, Dst: dst}
	if _, err := cloneOne(r, false); err != nil {
		t.Fatalf("setup clone failed: %v", err)
	}

	// push a new commit to bare from local
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", "update")
	cmd.Dir = local
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=t@t.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=t@t.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %s", out)
	}
	if out, err := exec.Command("git", "-C", local, "push").CombinedOutput(); err != nil {
		t.Fatalf("push: %s", out)
	}

	r.Dst = dst
	result, err := updateOne(r, false)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if result != "ok" {
		t.Fatalf("want 'ok', got %q", result)
	}
}

func TestUpdateOne_Shallow(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	bare := makeBareRepo(t)
	makeLocalRepo(t, bare)

	dst := filepath.Join(t.TempDir(), "shallow")
	r := config.RepoEntry{Name: "test", Repo: "file://" + bare, Dst: dst, Shallow: true}
	if _, err := cloneOne(r, false); err != nil {
		t.Fatalf("clone: %v", err)
	}
	result, err := updateOne(r, false)
	if err != nil {
		t.Fatalf("shallow update failed: %v", err)
	}
	if result != "ok" {
		t.Fatalf("want 'ok', got %q", result)
	}
}

// ---------- repoState ----------

func TestRepoState_NotExists(t *testing.T) {
	r := config.RepoEntry{Name: "test", Dst: filepath.Join(t.TempDir(), "nope")}
	s, err := repoState(r)
	if err != nil {
		t.Fatal(err)
	}
	if s.Exists {
		t.Fatal("repo should not exist")
	}
}

func TestRepoState_Exists(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	bare := makeBareRepo(t)
	makeLocalRepo(t, bare)

	dst := filepath.Join(t.TempDir(), "repo")
	r := config.RepoEntry{Name: "test", Repo: "file://" + bare, Dst: dst}
	if _, err := cloneOne(r, false); err != nil {
		t.Fatalf("setup: %v", err)
	}

	s, err := repoState(r)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Exists {
		t.Fatal("repo should exist")
	}
	if s.Current == "" {
		t.Fatal("current branch should be set")
	}
}
