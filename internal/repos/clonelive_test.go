package repos

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/ui"
)

// TestCloneAll_OrderAndActions clones two fresh repos and finds one already
// present, concurrently, and checks the results stay in config order with the
// expected actions.
func TestCloneAll_OrderAndActions(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	bare := makeBareRepo(t)
	makeLocalRepo(t, bare)

	tmp := t.TempDir()
	// A repo that already exists on disk (present).
	present := filepath.Join(tmp, "present")
	if err := os.MkdirAll(filepath.Join(present, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{Repos: []config.RepoEntry{
		{Name: "first", Repo: "file://" + bare, Dst: filepath.Join(tmp, "first")},
		{Name: "present", Repo: "file://" + bare, Dst: present},
		{Name: "second", Repo: "file://" + bare, Dst: filepath.Join(tmp, "second")},
	}}

	results, err := CloneAll(cfg, nil, false, ui.DiscardProgress(), 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"cloned", "present", "cloned"}
	if len(results) != len(want) {
		t.Fatalf("got %d results, want %d", len(results), len(want))
	}
	for i, w := range want {
		if results[i].Entry.Name != cfg.Repos[i].Name {
			t.Errorf("result %d out of order: got %q", i, results[i].Entry.Name)
		}
		if results[i].Action != w {
			t.Errorf("repo %q action: got %q, want %q", cfg.Repos[i].Name, results[i].Action, w)
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, "first", ".git")); err != nil {
		t.Errorf("first repo not cloned: %v", err)
	}
}

// TestCloneAll_DryRunNoSideEffects predicts clones without touching disk.
func TestCloneAll_DryRunNoSideEffects(t *testing.T) {
	tmp := t.TempDir()
	dst := filepath.Join(tmp, "new")
	cfg := config.Config{Repos: []config.RepoEntry{
		{Name: "new", Repo: "user/new", Dst: dst},
	}}

	results, err := CloneAll(cfg, nil, true, ui.DiscardProgress(), 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results[0].Action != "cloned" {
		t.Errorf("dry-run action: got %q, want %q", results[0].Action, "cloned")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("dry-run created %s", dst)
	}
}

// TestCloneAll_FailureRecorded reports a non-git directory as a failed result
// and returns it as the first error.
func TestCloneAll_FailureRecorded(t *testing.T) {
	tmp := t.TempDir()
	notGit := filepath.Join(tmp, "occupied")
	if err := os.MkdirAll(notGit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(notGit, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Repos: []config.RepoEntry{
		{Name: "occupied", Repo: "user/occupied", Dst: notGit},
	}}

	results, err := CloneAll(cfg, nil, false, ui.DiscardProgress(), 2)
	if err == nil {
		t.Fatal("expected an error for a non-git directory")
	}
	if results[0].Action != "failed" || results[0].Err == nil {
		t.Errorf("want failed result with Err, got %+v", results[0])
	}
}
