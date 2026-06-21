package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/shell"
)

// ── init ──────────────────────────────────────────────────────────────────────

func TestRunInit_CreatesScaffold(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "myrepo")

	if err := runInit(target); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	for _, sub := range []string{"files", "files.d", "shell"} {
		if fi, err := os.Stat(filepath.Join(target, sub)); err != nil || !fi.IsDir() {
			t.Errorf("expected directory %s", sub)
		}
	}

	data, err := os.ReadFile(filepath.Join(target, "dots.toml"))
	if err != nil {
		t.Fatalf("dots.toml not created: %v", err)
	}
	if !strings.Contains(string(data), "[meta]") {
		t.Error("dots.toml missing [meta] section")
	}
	if !strings.Contains(string(data), "[shell]") {
		t.Error("dots.toml missing [shell] section")
	}
}

func TestRunInit_ExistingTomlErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dots.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runInit(dir)
	if err == nil {
		t.Fatal("expected error for existing dots.toml")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunInit_CurrentDirDefault(t *testing.T) {
	dir := t.TempDir()
	if err := runInit(dir); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dots.toml")); err != nil {
		t.Error("dots.toml not created in target dir")
	}
}

// ── add ───────────────────────────────────────────────────────────────────────

func makeTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dots.toml"), []byte("[meta]\nversion = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunAdd_AdoptsFile(t *testing.T) {
	repoRoot := makeTestRepo(t)
	srcFile := filepath.Join(t.TempDir(), "myfile.conf")
	if err := os.WriteFile(srcFile, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := globals.cfg
	globals.cfg.RepoRoot = repoRoot
	t.Cleanup(func() { globals.cfg = orig })

	if err := runAdd(srcFile, ""); err != nil {
		t.Fatalf("runAdd: %v", err)
	}

	dest := filepath.Join(repoRoot, "files", "myfile.conf")
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("adopted file not found at %s", dest)
	}

	tomlData, _ := os.ReadFile(filepath.Join(repoRoot, "dots.toml"))
	if !strings.Contains(string(tomlData), "[[file]]") {
		t.Error("[[file]] entry not appended to dots.toml")
	}
	if !strings.Contains(string(tomlData), "files/myfile.conf") {
		t.Error("src not written to dots.toml")
	}
}

func TestRunAdd_Idempotent(t *testing.T) {
	repoRoot := makeTestRepo(t)
	srcFile := filepath.Join(t.TempDir(), "myfile.conf")
	if err := os.WriteFile(srcFile, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := globals.cfg
	globals.cfg.RepoRoot = repoRoot
	t.Cleanup(func() { globals.cfg = orig })

	if err := runAdd(srcFile, ""); err != nil {
		t.Fatalf("first runAdd: %v", err)
	}
	// Second add must not append a duplicate [[file]] block (invariant 3).
	if err := runAdd(srcFile, ""); err != nil {
		t.Fatalf("second runAdd: %v", err)
	}

	tomlData, _ := os.ReadFile(filepath.Join(repoRoot, "dots.toml"))
	if n := strings.Count(string(tomlData), "[[file]]"); n != 1 {
		t.Errorf("[[file]] entry written %d times, want 1", n)
	}
}

func TestRunAdd_MissingFile(t *testing.T) {
	repoRoot := makeTestRepo(t)
	orig := globals.cfg
	globals.cfg.RepoRoot = repoRoot
	t.Cleanup(func() { globals.cfg = orig })

	err := runAdd("/nonexistent/path.conf", "")
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestRunAdd_CustomDest(t *testing.T) {
	repoRoot := makeTestRepo(t)
	srcFile := filepath.Join(t.TempDir(), "rc")
	if err := os.WriteFile(srcFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := globals.cfg
	globals.cfg.RepoRoot = repoRoot
	t.Cleanup(func() { globals.cfg = orig })

	if err := runAdd(srcFile, "files.d/linux/rc"); err != nil {
		t.Fatalf("runAdd: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "files.d", "linux", "rc")); err != nil {
		t.Error("file not placed at custom dest")
	}
}

// ── migrate ───────────────────────────────────────────────────────────────────

func TestRunMigrate_NoFiles(t *testing.T) {
	repoRoot := makeTestRepo(t)
	orig := globals.cfg
	globals.cfg.RepoRoot = repoRoot
	t.Cleanup(func() { globals.cfg = orig })

	// With an empty home temp dir, nothing matches — just ensure no crash.
	// We can't easily override HOME without affecting other tests, so just
	// verify the function returns nil when the scan finds nothing.
	if err := runMigrate(false, ""); err != nil {
		t.Fatalf("runMigrate: %v", err)
	}
}

func TestRunMigrate_Write(t *testing.T) {
	repoRoot := makeTestRepo(t)

	// Plant a fake home with a known file that migrateScan will find.
	fakeHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(fakeHome, ".vimrc"), []byte("set nu"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := globals.cfg
	origHome := os.Getenv("HOME")
	globals.cfg.RepoRoot = repoRoot
	os.Setenv("HOME", fakeHome)
	t.Cleanup(func() {
		globals.cfg = orig
		os.Setenv("HOME", origHome)
	})

	if err := runMigrate(true, ""); err != nil {
		t.Fatalf("runMigrate --write: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, "files", ".vimrc")); err != nil {
		t.Error("expected .vimrc copied to repo")
	}
	tomlData, _ := os.ReadFile(filepath.Join(repoRoot, "dots.toml"))
	if !strings.Contains(string(tomlData), "[[file]]") {
		t.Error("expected [[file]] entry in dots.toml")
	}
}

// ── apply orchestration ───────────────────────────────────────────────────────

// makeShellCfg returns a minimal Config with shell.managed=true pointing at
// temp dirs so tests do not touch the real ~/.zshrc or shell.d.
func makeShellCfg(t *testing.T) config.Config {
	t.Helper()
	home := t.TempDir()
	zshrc := filepath.Join(home, ".zshrc")
	bashrc := filepath.Join(home, ".bashrc")
	shellDir := filepath.Join(home, ".config", "dots", "shell.d")
	if err := os.WriteFile(zshrc, []byte("# zshrc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bashrc, []byte("# bashrc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return config.Config{
		Shell: config.ShellConfig{
			Managed: true,
			Zshrc:   zshrc,
			Bashrc:  bashrc,
			Dir:     shellDir,
		},
	}
}

func TestApplyShell_WritesSnippetsAndBootstrapper(t *testing.T) {
	cfg := makeShellCfg(t)

	if err := applyShell(cfg, false); err != nil {
		t.Fatalf("applyShell: %v", err)
	}

	shellDir := cfg.Shell.Dir
	if _, err := os.Stat(filepath.Join(shellDir, "010-env.sh")); err != nil {
		t.Error("010-env.sh not written")
	}
	if _, err := os.Stat(filepath.Join(shellDir, "020-path.sh")); err != nil {
		t.Error("020-path.sh not written")
	}

	zshData, _ := os.ReadFile(cfg.Shell.Zshrc)
	if !strings.Contains(string(zshData), shell.MarkerStart) {
		t.Error("bootstrapper marker not inserted into zshrc")
	}
}

func TestApplyShell_DryRunNoWrites(t *testing.T) {
	cfg := makeShellCfg(t)

	if err := applyShell(cfg, true); err != nil {
		t.Fatalf("applyShell --dry-run: %v", err)
	}

	if _, err := os.Stat(cfg.Shell.Dir); err == nil {
		t.Error("shell.d directory should not exist in dry-run")
	}
	zshData, _ := os.ReadFile(cfg.Shell.Zshrc)
	if strings.Contains(string(zshData), shell.MarkerStart) {
		t.Error("bootstrapper should not be inserted in dry-run")
	}
}

func TestApplyShell_Idempotent(t *testing.T) {
	cfg := makeShellCfg(t)

	if err := applyShell(cfg, false); err != nil {
		t.Fatalf("first applyShell: %v", err)
	}
	if err := applyShell(cfg, false); err != nil {
		t.Fatalf("second applyShell: %v", err)
	}

	zshData, _ := os.ReadFile(cfg.Shell.Zshrc)
	count := strings.Count(string(zshData), shell.MarkerStart)
	if count != 1 {
		t.Errorf("bootstrapper inserted %d times, want 1", count)
	}
}

func TestApplyShell_SkipsWhenNotManaged(t *testing.T) {
	cfg := makeShellCfg(t)
	cfg.Shell.Managed = false

	if err := applyShell(cfg, false); err != nil {
		t.Fatalf("applyShell: %v", err)
	}

	if _, err := os.Stat(cfg.Shell.Dir); err == nil {
		t.Error("shell.d should not be created when shell.managed=false")
	}
}

func TestApplyPresets_FzfWritesToShellD(t *testing.T) {
	cfg := makeShellCfg(t)
	cfg.Presets.Fzf = true

	if err := applyPresets(cfg, false); err != nil {
		t.Fatalf("applyPresets: %v", err)
	}

	// A per-shell snippet is written for each shell; both bootstrappers source
	// only their matching suffix, so the key-binding paths must match the shell.
	for _, shellName := range []string{"zsh", "bash"} {
		fzfSnippet := filepath.Join(cfg.Shell.Dir, "030-fzf."+shellName)
		data, err := os.ReadFile(fzfSnippet)
		if err != nil {
			t.Fatalf("030-fzf.%s not written: %v", shellName, err)
		}
		if !strings.Contains(string(data), "fzf") {
			t.Errorf("030-fzf.%s does not look like an fzf snippet", shellName)
		}
		if !strings.Contains(string(data), "key-bindings."+shellName) {
			t.Errorf("030-fzf.%s missing %s-specific key bindings", shellName, shellName)
		}
	}
}

func TestApplyPresets_TmuxWritesConfig(t *testing.T) {
	home := t.TempDir()
	tmuxConf := filepath.Join(home, ".tmux.conf")

	origExpand := os.Getenv("HOME")
	os.Setenv("HOME", home)
	t.Cleanup(func() { os.Setenv("HOME", origExpand) })

	cfg := config.Config{}
	cfg.Presets.Tmux = true

	if err := applyPresets(cfg, false); err != nil {
		t.Fatalf("applyPresets: %v", err)
	}

	data, err := os.ReadFile(tmuxConf)
	if err != nil {
		t.Fatalf("~/.tmux.conf not written: %v", err)
	}
	if len(data) == 0 {
		t.Error("~/.tmux.conf is empty")
	}
}

func TestWriteUserFile_BacksUpForeignFile(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, ".tmux.conf")
	original := "# my hand-written tmux config\nset -g mouse on\n"
	if err := os.WriteFile(dst, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	action, err := writeUserFile(dst, "# new generated content\n")
	if err != nil {
		t.Fatalf("writeUserFile: %v", err)
	}
	if action != "backed up & wrote" {
		t.Errorf("action = %q, want %q", action, "backed up & wrote")
	}

	bak, err := os.ReadFile(dst + ".dots-bak")
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if string(bak) != original {
		t.Error("backup does not preserve the original content")
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "# new generated content\n" {
		t.Error("destination not overwritten with new content")
	}
}

func TestWriteUserFile_SkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, ".profile")
	content := "# unchanged content\n"
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	action, err := writeUserFile(dst, content)
	if err != nil {
		t.Fatalf("writeUserFile: %v", err)
	}
	if action != "unchanged" {
		t.Errorf("action = %q, want %q", action, "unchanged")
	}
	if _, err := os.Stat(dst + ".dots-bak"); !os.IsNotExist(err) {
		t.Error("unchanged write must not create a backup")
	}
}

func TestWriteUserFile_NoBackupForGenerated(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, ".zprofile")
	if err := os.WriteFile(dst, []byte(shell.GeneratedHeader+"\nold\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	action, err := writeUserFile(dst, shell.GeneratedHeader+"\nnew\n")
	if err != nil {
		t.Fatalf("writeUserFile: %v", err)
	}
	if action != "wrote" {
		t.Errorf("action = %q, want %q", action, "wrote")
	}
	if _, err := os.Stat(dst + ".dots-bak"); !os.IsNotExist(err) {
		t.Error("dots-generated file must be overwritten without a backup")
	}
}

func TestApplyPresets_DryRunNoWrites(t *testing.T) {
	cfg := makeShellCfg(t)
	cfg.Presets.Fzf = true

	if err := applyPresets(cfg, true); err != nil {
		t.Fatalf("applyPresets --dry-run: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cfg.Shell.Dir, "030-fzf.sh")); err == nil {
		t.Error("030-fzf.sh should not exist in dry-run")
	}
}

func TestRunApply_FilesOnly_SkipsSubsystems(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // keep the file deploy off the real home
	repoRoot := makeTestRepo(t)
	cfg := makeShellCfg(t)
	cfg.RepoRoot = repoRoot
	// A managed file the arg can match, deployed under the fake home.
	if err := os.WriteFile(filepath.Join(repoRoot, "files", ".testrc"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Shell is managed but apply is called with file args — subsystems should be skipped.
	// Verify bootstrapper is NOT inserted since filesOnly=true.
	if err := runApply(cfg, []string{".testrc"}, false, false, false); err != nil {
		t.Fatalf("runApply filesOnly: %v", err)
	}
	zshData, _ := os.ReadFile(cfg.Shell.Zshrc)
	if strings.Contains(string(zshData), shell.MarkerStart) {
		t.Error("bootstrapper should not be inserted when apply is called with file args")
	}
}

func TestRunApply_NoFiles_RunsShell(t *testing.T) {
	repoRoot := makeTestRepo(t)
	cfg := makeShellCfg(t)
	cfg.RepoRoot = repoRoot

	if err := runApply(cfg, nil, false, false, false); err != nil {
		t.Fatalf("runApply: %v", err)
	}

	zshData, _ := os.ReadFile(cfg.Shell.Zshrc)
	if !strings.Contains(string(zshData), shell.MarkerStart) {
		t.Error("bootstrapper should be inserted when apply runs with no file args")
	}
}

// F16: a file arg matching nothing is a typo — apply must fail, not report
// a misleading "0 linked" success.
func TestRunApply_UnmatchedArg_Errors(t *testing.T) {
	repoRoot := makeTestRepo(t)
	cfg := makeShellCfg(t)
	cfg.RepoRoot = repoRoot

	err := runApply(cfg, []string{"definitely-not-managed"}, false, false, false)
	if err == nil {
		t.Fatal("expected an error for an unmatched file arg")
	}
	if !strings.Contains(err.Error(), "no managed file matches") {
		t.Errorf("error: got %q, want it to mention no managed file matches", err)
	}
}

// F16: tools check/install must reject unknown tool names rather than silently
// matching nothing.
func TestCheckKnownTools(t *testing.T) {
	configured := []config.Tool{{Name: "fzf"}, {Name: "ripgrep"}}
	if err := checkKnownTools(configured, []string{"fzf"}); err != nil {
		t.Errorf("known tool should not error: %v", err)
	}
	err := checkKnownTools(configured, []string{"fzf", "bogus"})
	if err == nil {
		t.Fatal("expected an error for an unknown tool name")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should name the unknown tool, got %q", err)
	}
}

// ── runStatus / runList / runDiff ─────────────────────────────────────────────

func TestRunList_EmptyRepo(t *testing.T) {
	repoRoot := makeTestRepo(t)
	cfg := config.Config{RepoRoot: repoRoot}

	// No files discovered — should run without error.
	if err := runList(cfg, false); err != nil {
		t.Fatalf("runList: %v", err)
	}
}

func TestRunStatus_EmptyRepo(t *testing.T) {
	repoRoot := makeTestRepo(t)
	cfg := config.Config{RepoRoot: repoRoot}

	if err := runStatus(cfg); err != nil {
		t.Fatalf("runStatus: %v", err)
	}
}

func TestRunDiff_NoFiles(t *testing.T) {
	repoRoot := makeTestRepo(t)
	cfg := config.Config{RepoRoot: repoRoot}

	if err := runDiff(cfg, ""); err != nil {
		t.Fatalf("runDiff: %v", err)
	}
}
