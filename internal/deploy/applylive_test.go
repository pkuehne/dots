package deploy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/deploy"
	"github.com/pkuehne/dots/internal/ui"
)

// TestApplyAllLive_OrderAndActions deploys a mix of entries concurrently and
// checks the results come back in entry order with the expected actions and the
// files actually deployed.
func TestApplyAllLive_OrderAndActions(t *testing.T) {
	root, opts := makeRepo(t, map[string]string{
		"files/a": "aaa",
		"files/b": "bbb",
		"files/c": "ccc",
	})
	_ = root

	entries := []config.FileEntry{
		entry("files/a", filepath.Join(opts.HomeDir, "a")),
		entry("files/b", filepath.Join(opts.HomeDir, "b")),
		entry("files/c", filepath.Join(opts.HomeDir, "c")),
	}

	results := deploy.ApplyAllLive(entries, opts, ui.DiscardProgress(), 4)
	if len(results) != len(entries) {
		t.Fatalf("got %d results, want %d", len(results), len(entries))
	}
	for i, r := range results {
		if r.Err != nil {
			t.Fatalf("entry %d: unexpected error: %v", i, r.Err)
		}
		if r.Entry.Src != entries[i].Src {
			t.Errorf("result %d out of order: got %q, want %q", i, r.Entry.Src, entries[i].Src)
		}
		if r.Action != "linked" {
			t.Errorf("entry %d action: got %q, want %q", i, r.Action, "linked")
		}
		if _, err := os.Lstat(r.Entry.Dst); err != nil {
			t.Errorf("entry %d not deployed: %v", i, err)
		}
	}
}

// TestApplyAllLive_Idempotent confirms a second concurrent pass reports the
// entries as unchanged rather than re-linking.
func TestApplyAllLive_Idempotent(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/a": "aaa"})
	entries := []config.FileEntry{entry("files/a", filepath.Join(opts.HomeDir, "a"))}

	deploy.ApplyAllLive(entries, opts, ui.DiscardProgress(), 2)
	results := deploy.ApplyAllLive(entries, opts, ui.DiscardProgress(), 2)
	if results[0].Action != "unchanged" {
		t.Errorf("second pass action: got %q, want %q", results[0].Action, "unchanged")
	}
}

// TestApplyAllLive_DryRunNoSideEffects confirms dry-run predicts actions and
// touches nothing on disk.
func TestApplyAllLive_DryRunNoSideEffects(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/a": "aaa"})
	opts.DryRun = true
	dst := filepath.Join(opts.HomeDir, "a")
	entries := []config.FileEntry{entry("files/a", dst)}

	results := deploy.ApplyAllLive(entries, opts, ui.DiscardProgress(), 2)
	if results[0].Action != "link" {
		t.Errorf("dry-run action: got %q, want %q", results[0].Action, "link")
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Errorf("dry-run created %s (err=%v)", dst, err)
	}
}

// TestApplyAllLive_StagesReported confirms a real deploy drives the task through
// its stages and completes, while a no-op (unchanged) reports no stages.
func TestApplyAllLive_StagesReported(t *testing.T) {
	_, opts := makeRepo(t, map[string]string{"files/a": "aaa"})
	entries := []config.FileEntry{entry("files/a", filepath.Join(opts.HomeDir, "a"))}

	rec := &recordProgress{}
	deploy.ApplyAllLive(entries, opts, rec, 1)
	task := rec.only(t)
	if len(task.stages) == 0 {
		t.Error("expected at least one stage for a real deploy")
	}
	if !task.done {
		t.Error("task was not completed")
	}
}

// recordProgress is a ui.Progress that records each task's stages and terminal
// state for assertions.
type recordProgress struct {
	tasks []*recordTask
}

func (p *recordProgress) Task(name string) ui.Task {
	t := &recordTask{name: name}
	p.tasks = append(p.tasks, t)
	return t
}
func (p *recordProgress) Wait() {}

func (p *recordProgress) only(t *testing.T) *recordTask {
	t.Helper()
	if len(p.tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(p.tasks))
	}
	return p.tasks[0]
}

type recordTask struct {
	name   string
	stages []string
	done   bool
	failed bool
}

func (t *recordTask) Stage(msg string)            { t.stages = append(t.stages, msg) }
func (t *recordTask) SetTotal(int64)              {}
func (t *recordTask) Advance(int64)               {}
func (t *recordTask) Write(p []byte) (int, error) { return len(p), nil }
func (t *recordTask) Done(string)                 { t.done = true }
func (t *recordTask) Fail(error)                  { t.failed = true }
