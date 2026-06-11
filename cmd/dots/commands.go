package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/deploy"
	"github.com/pkuehne/dots/internal/discovery"
	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/fileutil"
	gogit "github.com/pkuehne/dots/internal/git"
	"github.com/pkuehne/dots/internal/platform"
	"github.com/pkuehne/dots/internal/presets"
	"github.com/pkuehne/dots/internal/repos"
	"github.com/pkuehne/dots/internal/secrets"
	"github.com/pkuehne/dots/internal/shell"
	gossh "github.com/pkuehne/dots/internal/ssh"
	"github.com/pkuehne/dots/internal/tools"
)

// ── init ─────────────────────────────────────────────────────────────────────

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold a new dots repository",
		Args:  cobra.MaximumNArgs(1),
		Annotations: map[string]string{"skipConfig": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return runInit(dir)
		},
	}
}

func runInit(dir string) error {
	d, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	tomlPath := filepath.Join(d, "dots.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		return errs.New(fmt.Sprintf("dots.toml already exists in %s", d),
			"Remove it first if you want to re-initialize.")
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	for _, sub := range []string{"files", "files.d", "shell"} {
		if err := os.MkdirAll(filepath.Join(d, sub), 0o755); err != nil {
			return err
		}
	}
	const tomlContent = "[meta]\nversion = 1\n\n[shell]\nmanaged = false\n\n[git]\nmanaged = false\n"
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o644); err != nil {
		return err
	}
	fmt.Printf("✓ Initialized dots in %s\n", d)
	fmt.Println("  Created: dots.toml, files/, files.d/, shell/")
	return nil
}

// ── apply / preview ──────────────────────────────────────────────────────────

func newApplyCmd() *cobra.Command {
	var dryRun, forceCopy bool

	cmd := &cobra.Command{
		Use:   "apply [files...]",
		Short: "Deploy files and generate managed configs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(globals.cfg, args, dryRun, forceCopy)
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "print actions without executing")
	cmd.Flags().BoolVarP(&forceCopy, "copy", "c", false, "force copy mode instead of symlink")
	return cmd
}

func runApply(cfg config.Config, fileArgs []string, dryRun, forceCopy bool) error {
	filesOnly := len(fileArgs) > 0

	opts := deploy.Options{
		DryRun:        dryRun,
		ForceCopy:     forceCopy,
		RepoRoot:      cfg.RepoRoot,
		DefaultMode:   cfg.Meta.DefaultMode,
		ActiveProfile: cfg.ActiveProfile,
	}

	entries, err := discovery.Walk(cfg, platform.Detect())
	if err != nil {
		return err
	}

	if filesOnly {
		var filtered []config.FileEntry
		for _, e := range entries {
			for _, a := range fileArgs {
				if strings.Contains(e.Src, a) || strings.Contains(e.Dst, a) ||
					filepath.Base(e.Dst) == a {
					filtered = append(filtered, e)
					break
				}
			}
		}
		entries = filtered
	}

	results := deploy.ApplyAll(entries, opts)
	printResults(results, dryRun)

	if filesOnly {
		return nil
	}

	if err := applyShell(cfg, dryRun); err != nil {
		return err
	}
	if err := applyGit(cfg, dryRun); err != nil {
		return err
	}
	if err := applySSH(cfg, dryRun); err != nil {
		return err
	}
	if err := applyRepos(cfg, dryRun); err != nil {
		return err
	}
	if err := applyPresets(cfg, dryRun); err != nil {
		return err
	}
	if err := applyTools(cfg, dryRun); err != nil {
		return err
	}
	if err := applyLoginShell(cfg, dryRun); err != nil {
		return err
	}
	return nil
}

func applyShell(cfg config.Config, dryRun bool) error {
	if !cfg.Shell.Managed {
		return nil
	}
	if err := shell.WriteSnippets(cfg, dryRun); err != nil {
		return err
	}
	return shell.InsertSourceLine(cfg, dryRun)
}

func applyGit(cfg config.Config, dryRun bool) error {
	if !cfg.Git.Managed {
		return nil
	}
	return gogit.WriteManaged(cfg, dryRun)
}

func applySSH(cfg config.Config, dryRun bool) error {
	if !cfg.SSH.Managed {
		return nil
	}
	return gossh.WriteManaged(cfg, platform.Detect(), dryRun)
}

func applyRepos(cfg config.Config, dryRun bool) error {
	if len(cfg.Repos) == 0 {
		return nil
	}
	return repos.Clone(cfg, nil, dryRun)
}

func applyPresets(cfg config.Config, dryRun bool) error {
	if cfg.Presets.Fzf && cfg.Shell.Managed {
		content, err := presets.Generate("fzf", cfg)
		if err != nil {
			return err
		}
		dir := fileutil.Expand(cfg.Shell.Dir)
		dst := filepath.Join(dir, "030-fzf.sh")
		if !dryRun {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
				return err
			}
			fmt.Printf("  wrote fzf preset → %s\n", dst)
		} else {
			fmt.Printf("  would write fzf preset → %s\n", dst)
		}
	}
	if cfg.Presets.Tmux {
		content, err := presets.Generate("tmux", cfg)
		if err != nil {
			return err
		}
		dst := fileutil.Expand("~/.tmux.conf")
		if !dryRun {
			if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
				return err
			}
			fmt.Printf("  wrote tmux preset → %s\n", dst)
		} else {
			fmt.Printf("  would write tmux preset → %s\n", dst)
		}
	}
	return nil
}

func applyTools(cfg config.Config, dryRun bool) error {
	if len(cfg.Tools) == 0 {
		return nil
	}
	plat := platform.Detect()
	arch := platform.Arch()
	results := tools.Check(cfg.Tools, plat, arch)
	opts := tools.InstallOptions{DryRun: dryRun}
	installErrors := 0
	for _, r := range results {
		if r.Installed {
			continue
		}
		if err := tools.Install(r.Tool, cfg, plat, arch, opts); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", r.Tool.Name, err)
			installErrors++
		} else if dryRun {
			fmt.Printf("  would install %s\n", r.Tool.Name)
		} else {
			fmt.Printf("  ✓ installed %s\n", r.Tool.Name)
		}
	}
	if installErrors > 0 {
		return errs.New(fmt.Sprintf("%d tool(s) failed to install", installErrors),
			"Run 'dots tools check' for details.")
	}
	return nil
}

func applyLoginShell(cfg config.Config, dryRun bool) error {
	if !cfg.Shell.Managed || !cfg.Shell.Login {
		return nil
	}
	zprofile := fileutil.Expand("~/.zprofile")
	profile := fileutil.Expand("~/.profile")
	if dryRun {
		fmt.Printf("  would write login shell: %s, %s\n", zprofile, profile)
		return nil
	}
	if err := os.WriteFile(zprofile, []byte(presets.GenerateZprofile(cfg)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(profile, []byte(presets.GenerateProfile(cfg)), 0o644); err != nil {
		return err
	}
	fmt.Printf("  wrote login shell: %s, %s\n", zprofile, profile)
	return nil
}

func printResults(results []deploy.Result, dryRun bool) {
	counts := map[string]int{}
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "  error  %s: %v\n", r.Entry.Dst, r.Err)
			counts["error"]++
			continue
		}
		counts[r.Action]++
		if r.Action == "skipped" || r.Action == "unchanged" {
			continue
		}
		verb := r.Action
		if dryRun {
			verb = "would " + verb
		}
		fmt.Printf("%9s  %s\n", verb, r.Entry.Dst)
	}
	fmt.Printf("\n%d linked, %d copied, %d unchanged, %d skipped",
		counts["linked"]+counts["link"],
		counts["copied"]+counts["copy"],
		counts["unchanged"],
		counts["skipped"],
	)
	if counts["missing"] > 0 {
		fmt.Printf(", %d missing", counts["missing"])
	}
	if counts["error"] > 0 {
		fmt.Printf(", %d errors", counts["error"])
	}
	fmt.Println()
}

func newPreviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "preview [files...]",
		Short: "Alias for apply --dry-run",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(globals.cfg, args, true, false)
		},
	}
}

// ── status / diff / list ─────────────────────────────────────────────────────

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show deployment state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(globals.cfg)
		},
	}
}

func runStatus(cfg config.Config) error {
	entries, err := discovery.Walk(cfg, platform.Detect())
	if err != nil {
		return err
	}
	opts := deploy.Options{
		RepoRoot:      cfg.RepoRoot,
		DefaultMode:   cfg.Meta.DefaultMode,
		ActiveProfile: cfg.ActiveProfile,
	}
	fmt.Println("Files:")
	for _, e := range entries {
		r := deploy.Status(e, opts)
		if r.Action == "skipped" {
			continue
		}
		icon := " "
		switch r.Action {
		case "linked", "copied", "unchanged":
			icon = "✓"
		case "missing":
			icon = "✗"
		case "diff":
			icon = "~"
		}
		fmt.Printf("  %s  %-10s  %s\n", icon, r.Action, e.Dst)
	}

	if len(cfg.Repos) > 0 {
		fmt.Println("\nRepos:")
		states, err := repos.Status(cfg)
		if err != nil {
			return err
		}
		for _, s := range states {
			icon := "?"
			state := "unknown"
			switch {
			case !s.Exists:
				icon, state = "✗", "missing"
			case s.Dirty:
				icon, state = "~", "dirty"
			case s.Behind > 0:
				icon, state = "↓", fmt.Sprintf("behind %d", s.Behind)
			default:
				icon, state = "✓", "ok"
			}
			fmt.Printf("  %s  %-10s  %s\n", icon, state, s.Entry.Dst)
		}
	}
	return nil
}

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff [file]",
		Short: "Show diffs between source and deployed files",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := ""
			if len(args) > 0 {
				filter = args[0]
			}
			return runDiff(globals.cfg, filter)
		},
	}
}

func runDiff(cfg config.Config, filter string) error {
	entries, err := discovery.Walk(cfg, platform.Detect())
	if err != nil {
		return err
	}
	opts := deploy.Options{
		RepoRoot:      cfg.RepoRoot,
		DefaultMode:   cfg.Meta.DefaultMode,
		ActiveProfile: cfg.ActiveProfile,
	}
	any := false
	for _, e := range entries {
		if filter != "" && !strings.Contains(e.Dst, filter) &&
			!strings.Contains(e.Src, filter) &&
			filepath.Base(e.Dst) != filter {
			continue
		}
		r := deploy.Status(e, opts)
		if r.Action != "diff" {
			continue
		}
		any = true
		src := filepath.Join(cfg.RepoRoot, e.Src)
		dst := fileutil.Expand(e.Dst)
		cmd := exec.Command("diff", "-u", dst, src)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run() // diff exits 1 when files differ — that's normal
	}
	if !any {
		fmt.Println("  No diffs found.")
	}
	return nil
}

func newListCmd() *cobra.Command {
	var showAll bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List managed files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(globals.cfg, showAll)
		},
	}
	cmd.Flags().BoolVar(&showAll, "all", false, "include skipped files")
	return cmd
}

func runList(cfg config.Config, showAll bool) error {
	entries, err := discovery.Walk(cfg, platform.Detect())
	if err != nil {
		return err
	}
	opts := deploy.Options{
		RepoRoot:      cfg.RepoRoot,
		DefaultMode:   cfg.Meta.DefaultMode,
		ActiveProfile: cfg.ActiveProfile,
	}
	for _, e := range entries {
		r := deploy.Status(e, opts)
		if r.Action == "skipped" && !showAll {
			continue
		}
		fmt.Printf("  %-12s  %s\n", r.Action, e.Dst)
	}
	return nil
}

// ── edit / add ───────────────────────────────────────────────────────────────

func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <file>",
		Short: "Open source file in $EDITOR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEdit(args[0])
		},
	}
}

func runEdit(fileArg string) error {
	entries, err := discovery.Walk(globals.cfg, platform.Detect())
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.Contains(e.Src, fileArg) ||
			strings.Contains(e.Dst, fileArg) ||
			strings.HasSuffix(e.Src, fileArg) ||
			filepath.Base(e.Dst) == fileArg {
			src := filepath.Join(globals.cfg.RepoRoot, e.Src)
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = os.Getenv("VISUAL")
			}
			if editor == "" {
				editor = "vi"
			}
			editorPath, err := exec.LookPath(editor)
			if err != nil {
				return errs.New(fmt.Sprintf("editor not found: %s", editor),
					"Set $EDITOR to a valid editor binary.")
			}
			return syscall.Exec(editorPath, []string{editor, src}, os.Environ())
		}
	}
	return errs.New(fmt.Sprintf("File not found: %s", fileArg),
		"Check dots list for available files.")
}

func newAddCmd() *cobra.Command {
	var dest string
	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Adopt an existing file into the repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(args[0], dest)
		},
	}
	cmd.Flags().StringVar(&dest, "dest", "", "override repo destination path")
	return cmd
}

func runAdd(path, dest string) error {
	src, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(src); err != nil {
		return errs.New(fmt.Sprintf("File not found: %s", path), "")
	}

	repoRoot := globals.cfg.RepoRoot
	var repoDest string
	if dest != "" {
		repoDest = filepath.Join(repoRoot, dest)
	} else {
		repoDest = filepath.Join(repoRoot, "files", filepath.Base(src))
	}

	if err := os.MkdirAll(filepath.Dir(repoDest), 0o755); err != nil {
		return err
	}
	if err := fileutil.CopyFile(src, repoDest); err != nil {
		return err
	}

	relSrc, err := filepath.Rel(repoRoot, repoDest)
	if err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	dstStr := src
	if strings.HasPrefix(dstStr, home) {
		dstStr = "~" + dstStr[len(home):]
	}

	tomlPath := filepath.Join(repoRoot, "dots.toml")
	f, err := os.OpenFile(tomlPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "\n[[file]]\nsrc = %q\ndst = %q\n", relSrc, dstStr)

	fmt.Printf("✓ Adopted %s → %s\n", path, relSrc)
	fmt.Println("  [[file]] entry added to dots.toml")
	return nil
}

// ── doctor / migrate ─────────────────────────────────────────────────────────

// sensitiveDirs mirrors the fileutil map for permission checks.
var sensitiveDirs = map[string]os.FileMode{
	".ssh":   0o700,
	".gnupg": 0o700,
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "System health check",
		RunE: func(cmd *cobra.Command, args []string) error {
			code := runDoctor()
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}
}

func runDoctor() int {
	warnings, errors := 0, 0

	ok := func(msg string) { fmt.Printf("  ✓ %s\n", msg) }
	warn := func(msg string) { warnings++; fmt.Printf("  ⚠ %s\n", msg) }
	fail := func(msg string) { errors++; fmt.Printf("  ✗ %s\n", msg) }

	fmt.Println("dots doctor")
	fmt.Println()

	cfg := globals.cfg

	// dots.toml
	tomlPath := filepath.Join(cfg.RepoRoot, "dots.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		ok("dots.toml found and parsed")
	} else {
		warn("dots.toml not found (zero-config mode)")
	}

	// git binary (if repos configured)
	if len(cfg.Repos) > 0 {
		if _, err := exec.LookPath("git"); err == nil {
			ok("git available (repos configured)")
		} else {
			fail("git not found but [[repo]] entries configured")
		}
	}

	// age binary (if .age files or secrets.recipient configured)
	ageFiles, _ := filepath.Glob(filepath.Join(cfg.RepoRoot, "**/*.age"))
	if len(ageFiles) == 0 {
		// filepath.Glob doesn't recurse; use filepath.WalkDir instead
		_ = filepath.WalkDir(cfg.RepoRoot, func(p string, d os.DirEntry, _ error) error {
			if strings.HasSuffix(p, ".age") {
				ageFiles = append(ageFiles, p)
			}
			return nil
		})
	}
	if len(ageFiles) > 0 || cfg.Secrets.Recipient != "" {
		if _, err := exec.LookPath("age"); err == nil {
			ok("age available (secrets configured)")
		} else {
			fail("age not found but .age files or [secrets] configured")
		}
	}

	// GITHUB_TOKEN (if tools configured)
	if len(cfg.Tools) > 0 {
		if os.Getenv("GITHUB_TOKEN") != "" {
			ok("GITHUB_TOKEN set")
		} else {
			warn("GITHUB_TOKEN not set — GitHub API rate limits apply (60 req/hr)")
		}
	}

	// ~/.local/bin on PATH
	localBin := fileutil.Expand("~/.local/bin")
	if strings.Contains(os.Getenv("PATH"), localBin) {
		ok("~/.local/bin on PATH")
	} else {
		warn("~/.local/bin not on PATH")
	}

	// Shell bootstrapper
	if cfg.Shell.Managed {
		zshrc := fileutil.Expand(cfg.Shell.Zshrc)
		data, err := os.ReadFile(zshrc)
		if err == nil && strings.Contains(string(data), shell.MarkerStart) {
			ok(fmt.Sprintf("Shell bootstrapper installed in %s", zshrc))
		} else {
			warn(fmt.Sprintf("Shell bootstrapper not found in %s", zshrc))
		}
	}

	// Git [include]
	if cfg.Git.Managed {
		gitconfig := fileutil.Expand("~/.gitconfig")
		data, err := os.ReadFile(gitconfig)
		if err == nil && strings.Contains(string(data), shell.MarkerStart) {
			ok("Git [include] present in ~/.gitconfig")
		} else {
			warn("Git [include] not found in ~/.gitconfig")
		}
	}

	// SSH Include
	if cfg.SSH.Managed {
		sshConfig := fileutil.Expand("~/.ssh/config")
		data, err := os.ReadFile(sshConfig)
		if err == nil && strings.Contains(string(data), gossh.IncludeLine) {
			ok("SSH Include present in ~/.ssh/config")
		} else {
			warn("SSH Include not found in ~/.ssh/config")
		}
	}

	// Sensitive dir permissions
	home, _ := os.UserHomeDir()
	for dir, expected := range sensitiveDirs {
		p := filepath.Join(home, dir)
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		actual := info.Mode().Perm()
		if actual == expected {
			ok(fmt.Sprintf("~/%s permissions: %o", dir, actual))
		} else {
			warn(fmt.Sprintf("~/%s permissions: %o (expected %o)", dir, actual, expected))
		}
	}

	// Disk space
	var st syscall.Statfs_t
	if err := syscall.Statfs(home, &st); err == nil {
		freeMB := int64(st.Bavail) * st.Bsize / (1024 * 1024)
		if freeMB < 100 {
			warn(fmt.Sprintf("Low disk space in $HOME: %d MB free", freeMB))
		} else {
			ok(fmt.Sprintf("Disk space: %d MB free", freeMB))
		}
	}

	fmt.Println()
	if errors > 0 {
		fmt.Printf("%d error(s), %d warning(s)\n", errors, warnings)
		return 1
	}
	if warnings > 0 {
		fmt.Printf("%d warning(s)\n", warnings)
		return 1
	}
	fmt.Println("All checks passed")
	return 0
}

// migrateScan is the well-known list of dotfiles to look for during migration.
var migrateScan = []string{
	".zshrc",
	".bashrc",
	".gitconfig",
	".vimrc",
	".tmux.conf",
	".ssh/config",
	".config/nvim/init.lua",
	".config/starship.toml",
	".config/alacritty/alacritty.yml",
	".config/alacritty/alacritty.toml",
	".config/kitty/kitty.conf",
	".config/wezterm/wezterm.lua",
}

func newMigrateCmd() *cobra.Command {
	var write bool
	var plat string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Scan for unmanaged dotfiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrate(write, plat)
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "copy files and add entries to dots.toml")
	cmd.Flags().StringVar(&plat, "platform", "", "target platform directory")
	return cmd
}

func runMigrate(write bool, plat string) error {
	home, _ := os.UserHomeDir()
	repoRoot := globals.cfg.RepoRoot

	managedSrcs := map[string]bool{}
	for _, e := range globals.cfg.Files {
		managedSrcs[e.Src] = true
	}

	type suggestion struct{ rel, destDir string }
	var suggestions []suggestion

	for _, rel := range migrateScan {
		fp := filepath.Join(home, filepath.FromSlash(rel))
		if _, err := os.Stat(fp); err != nil {
			continue
		}
		candidateSrc := "files/" + rel
		if managedSrcs[candidateSrc] {
			continue
		}
		if _, err := os.Stat(filepath.Join(repoRoot, candidateSrc)); err == nil {
			continue
		}

		// Already a symlink into the repo?
		if target, err := os.Readlink(fp); err == nil {
			if strings.HasPrefix(target, repoRoot) {
				fmt.Printf("  ✓ ~/%s — already symlinked into repo\n", rel)
				continue
			}
		}

		destDir := "files/"
		if plat != "" {
			destDir = "files.d/" + plat + "/"
		}
		fmt.Printf("  Found: ~/%s\n", rel)
		fmt.Println("    Suggested [[file]] entry:")
		fmt.Printf("    src  = %q\n    dst  = %q\n\n", destDir+rel, "~/"+rel)
		suggestions = append(suggestions, suggestion{rel, destDir})
	}

	if len(suggestions) == 0 {
		fmt.Println("  No unmanaged dotfiles found to migrate.")
		return nil
	}

	if !write {
		return nil
	}

	for _, s := range suggestions {
		src := filepath.Join(home, filepath.FromSlash(s.rel))
		repoDest := filepath.Join(repoRoot, filepath.FromSlash(s.destDir), filepath.FromSlash(s.rel))
		if err := os.MkdirAll(filepath.Dir(repoDest), 0o755); err != nil {
			return err
		}
		if err := fileutil.CopyFile(src, repoDest); err != nil {
			return err
		}
		rel, _ := filepath.Rel(repoRoot, repoDest)
		fmt.Printf("  Copied ~/%s → %s\n", s.rel, rel)
	}

	tomlPath := filepath.Join(repoRoot, "dots.toml")
	f, err := os.OpenFile(tomlPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "\n# Migrated files")
	for _, s := range suggestions {
		fmt.Fprintf(f, "\n[[file]]\nsrc = %q\ndst = %q\n",
			s.destDir+s.rel, "~/"+s.rel)
	}
	fmt.Println("\n  Entries appended to dots.toml")
	return nil
}

// ── encrypt / decrypt ────────────────────────────────────────────────────────

func newEncryptCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "encrypt <file>",
		Short: "Encrypt a file with age",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]
			if _, err := os.Stat(src); err != nil {
				return errs.New(fmt.Sprintf("File not found: %s", src), "")
			}
			dst := output
			if dst == "" {
				dst = src + ".age"
			}
			if err := secrets.Encrypt(src, dst, globals.cfg); err != nil {
				return err
			}
			fmt.Printf("✓ Encrypted %s → %s\n", src, dst)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output path (default: <file>.age)")
	return cmd
}

func newDecryptCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "decrypt <file>",
		Short: "Decrypt an .age file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src := args[0]
			if _, err := os.Stat(src); err != nil {
				return errs.New(fmt.Sprintf("File not found: %s", src), "")
			}
			if !strings.HasSuffix(src, ".age") {
				return errs.New(fmt.Sprintf("File must end in .age: %s", src), "")
			}
			dst := output
			if dst == "" {
				dst = strings.TrimSuffix(src, ".age")
			}
			if err := secrets.Decrypt(src, dst, globals.cfg); err != nil {
				return err
			}
			fmt.Printf("✓ Decrypted %s → %s\n", src, dst)
			return nil
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output path (default: file without .age)")
	return cmd
}

// ── tools ────────────────────────────────────────────────────────────────────

func newToolsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Manage tool installations",
	}

	var checkTag string
	check := &cobra.Command{
		Use:   "check [names...]",
		Short: "Check which configured tools are installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			plat := platform.Detect()
			arch := platform.Arch()
			filtered := tools.Filter(globals.cfg.Tools, args, checkTag)
			if len(filtered) == 0 {
				fmt.Println("  No tools configured.")
				return nil
			}
			results := tools.Check(filtered, plat, arch)
			installed := 0
			for _, r := range results {
				icon := "✗"
				if r.Installed {
					icon = "✓"
					installed++
				}
				fmt.Printf("  %s  %s\n", icon, r.Tool.Name)
			}
			fmt.Printf("\n%d/%d tools installed\n", installed, len(results))
			return nil
		},
	}
	check.Flags().StringVar(&checkTag, "tag", "", "filter by tag")

	var installTag string
	var installDryRun, installForce bool
	install := &cobra.Command{
		Use:   "install [names...]",
		Short: "Install missing tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			plat := platform.Detect()
			arch := platform.Arch()
			filtered := tools.Filter(globals.cfg.Tools, args, installTag)
			if !installForce {
				results := tools.Check(filtered, plat, arch)
				var missing []config.Tool
				for _, r := range results {
					if !r.Installed {
						missing = append(missing, r.Tool)
					}
				}
				filtered = missing
			}
			if len(filtered) == 0 {
				fmt.Println("  All tools are already installed.")
				return nil
			}
			opts := tools.InstallOptions{DryRun: installDryRun}
			installErrors := 0
			for _, t := range filtered {
				if err := tools.Install(t, globals.cfg, plat, arch, opts); err != nil {
					fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", t.Name, err)
					installErrors++
				} else if installDryRun {
					fmt.Printf("  would install %s\n", t.Name)
				} else {
					fmt.Printf("  ✓ installed %s\n", t.Name)
				}
			}
			if installErrors > 0 {
				return errs.New(fmt.Sprintf("%d tool(s) failed to install", installErrors),
					"Check the error messages above for details.")
			}
			return nil
		},
	}
	install.Flags().StringVar(&installTag, "tag", "", "filter by tag")
	install.Flags().BoolVarP(&installDryRun, "dry-run", "n", false, "print actions without executing")
	install.Flags().BoolVarP(&installForce, "force", "f", false, "reinstall even if present")

	var listTag string
	list := &cobra.Command{
		Use:   "list",
		Short: "List configured tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			filtered := tools.Filter(globals.cfg.Tools, args, listTag)
			if len(filtered) == 0 {
				fmt.Println("  No tools configured.")
				return nil
			}
			for _, t := range filtered {
				tags := ""
				if len(t.Tags) > 0 {
					tags = " [" + strings.Join(t.Tags, ", ") + "]"
				}
				fmt.Printf("  %-20s%s\n", t.Name, tags)
			}
			return nil
		},
	}
	list.Flags().StringVar(&listTag, "tag", "", "filter by tag")

	cmd.AddCommand(check, install, list)
	return cmd
}

// ── shell ────────────────────────────────────────────────────────────────────

func newShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Manage shell integration",
	}

	var assembled bool
	show := &cobra.Command{
		Use:   "show",
		Short: "Print generated shell snippets",
		RunE: func(cmd *cobra.Command, args []string) error {
			if assembled {
				content, err := shell.Assembled(globals.cfg)
				if err != nil {
					return err
				}
				fmt.Print(content)
				return nil
			}
			fmt.Print(shell.GenerateEnvSnippet(globals.cfg))
			fmt.Print(shell.GeneratePathSnippet(globals.cfg))
			return nil
		},
	}
	show.Flags().BoolVar(&assembled, "assembled", false, "print full assembled output from shell.d")

	var cleanDryRun bool
	clean := &cobra.Command{
		Use:   "clean",
		Short: "Remove stale snippets",
		RunE: func(cmd *cobra.Command, args []string) error {
			return shell.Clean(globals.cfg, cleanDryRun)
		},
	}
	clean.Flags().BoolVarP(&cleanDryRun, "dry-run", "n", false, "print actions without executing")

	cmd.AddCommand(show, clean)
	return cmd
}

// ── repos ────────────────────────────────────────────────────────────────────

func newReposCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "repos", Short: "Manage git repositories"}

	var cloneDryRun bool
	clone := &cobra.Command{
		Use:   "clone [names...]",
		Short: "Clone missing repos",
		RunE: func(cmd *cobra.Command, args []string) error {
			return repos.Clone(globals.cfg, args, cloneDryRun)
		},
	}
	clone.Flags().BoolVarP(&cloneDryRun, "dry-run", "n", false, "print actions without executing")

	var updateDryRun bool
	update := &cobra.Command{
		Use:   "update [names...]",
		Short: "Update cloned repos",
		RunE: func(cmd *cobra.Command, args []string) error {
			return repos.Update(globals.cfg, args, updateDryRun)
		},
	}
	update.Flags().BoolVarP(&updateDryRun, "dry-run", "n", false, "print actions without executing")

	status := &cobra.Command{
		Use:   "status",
		Short: "Show repo states",
		RunE: func(cmd *cobra.Command, args []string) error {
			states, err := repos.Status(globals.cfg)
			if err != nil {
				return err
			}
			if len(states) == 0 {
				fmt.Println("  No repos configured.")
				return nil
			}
			for _, s := range states {
				icon, state := "✓", "ok"
				switch {
				case !s.Exists:
					icon, state = "✗", "missing"
				case s.Dirty:
					icon, state = "~", "dirty"
				case s.Behind > 0:
					icon, state = "↓", fmt.Sprintf("behind %d", s.Behind)
				}
				fmt.Printf("  %s  %-12s  %s\n", icon, state, s.Entry.Dst)
			}
			return nil
		},
	}

	cmd.AddCommand(clone, update, status)
	return cmd
}

// ── git ──────────────────────────────────────────────────────────────────────

func newGitCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "git", Short: "Manage git config"}

	var initDryRun bool
	initSub := &cobra.Command{
		Use:   "init",
		Short: "Enable git managed mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gogit.WriteManaged(globals.cfg, initDryRun)
		},
	}
	initSub.Flags().BoolVarP(&initDryRun, "dry-run", "n", false, "print actions without executing")

	show := &cobra.Command{
		Use:   "show",
		Short: "Print managed.gitconfig",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gogit.ShowManaged(globals.cfg)
		},
	}

	var uninitDryRun bool
	uninit := &cobra.Command{
		Use:   "uninit",
		Short: "Remove dots [include] from ~/.gitconfig",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gogit.Uninit(globals.cfg, uninitDryRun)
		},
	}
	uninit.Flags().BoolVarP(&uninitDryRun, "dry-run", "n", false, "print actions without executing")

	cmd.AddCommand(initSub, show, uninit)
	return cmd
}

// ── ssh ──────────────────────────────────────────────────────────────────────

func newSSHCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "ssh", Short: "Manage SSH config"}

	var initDryRun bool
	initSub := &cobra.Command{
		Use:   "init",
		Short: "Enable SSH managed mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gossh.WriteManaged(globals.cfg, platform.Detect(), initDryRun)
		},
	}
	initSub.Flags().BoolVarP(&initDryRun, "dry-run", "n", false, "print actions without executing")

	show := &cobra.Command{
		Use:   "show",
		Short: "Print SSH config fragment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gossh.ShowManaged(globals.cfg, platform.Detect())
		},
	}

	var uninitDryRun bool
	uninit := &cobra.Command{
		Use:   "uninit",
		Short: "Remove dots Include from ~/.ssh/config",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gossh.Uninit(globals.cfg, uninitDryRun)
		},
	}
	uninit.Flags().BoolVarP(&uninitDryRun, "dry-run", "n", false, "print actions without executing")

	cmd.AddCommand(initSub, show, uninit)
	return cmd
}

// ── env ──────────────────────────────────────────────────────────────────────

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "env", Short: "Manage environment variables"}

	show := &cobra.Command{
		Use:   "show",
		Short: "Print 010-env.sh content",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print(shell.GenerateEnvSnippet(globals.cfg))
			return nil
		},
	}

	check := &cobra.Command{
		Use:   "check",
		Short: "Check [[env.when]] conditions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvCheck(globals.cfg)
		},
	}

	cmd.AddCommand(show, check)
	return cmd
}

func runEnvCheck(cfg config.Config) error {
	if len(cfg.Env.When) == 0 {
		fmt.Println("  No [[env.when]] conditions configured.")
		return nil
	}
	plats := platform.Platforms()
	for _, w := range cfg.Env.When {
		active := true
		var reasons []string
		if len(w.Only) > 0 {
			match := false
		outer:
			for _, want := range w.Only {
				for _, have := range plats {
					if want == have {
						match = true
						break outer
					}
				}
			}
			if !match {
				active = false
				reasons = append(reasons, fmt.Sprintf("platform %v not in %v", plats, w.Only))
			}
		}
		if w.IfTool != "" {
			if _, err := exec.LookPath(w.IfTool); err != nil {
				active = false
				reasons = append(reasons, fmt.Sprintf("tool %s not found", w.IfTool))
			}
		}
		icon := "✓"
		if !active {
			icon = "✗"
		}
		line := fmt.Sprintf("  %s  %s=%s", icon, w.Key, w.Value)
		if len(reasons) > 0 {
			line += "  (" + strings.Join(reasons, "; ") + ")"
		}
		fmt.Println(line)
	}
	return nil
}

// ── presets ──────────────────────────────────────────────────────────────────

func newPresetsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "presets", Short: "Manage presets"}

	show := &cobra.Command{
		Use:   "show <preset>",
		Short: "Print preset output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			content, err := presets.Generate(args[0], globals.cfg)
			if err != nil {
				return err
			}
			fmt.Print(content)
			return nil
		},
	}

	var ejectDest string
	eject := &cobra.Command{
		Use:   "eject <preset>",
		Short: "Eject preset to plain files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			dest := ejectDest
			if dest == "" {
				dest = fileutil.Expand(fmt.Sprintf("~/.config/dots/ejected/%s", name))
			}
			if err := presets.Eject(name, dest, globals.cfg); err != nil {
				return err
			}
			fmt.Printf("✓ Ejected %s → %s\n", name, dest)
			fmt.Println("  Remove the preset flag from dots.toml to stop managing it.")
			return nil
		},
	}
	eject.Flags().StringVar(&ejectDest, "dest", "", "output path (default: ~/.config/dots/ejected/<preset>)")

	cmd.AddCommand(show, eject)
	return cmd
}

// ── completion ────────────────────────────────────────────────────────────────

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion script",
		Long: `Generate a shell completion script for dots and print it to stdout.

To load completions in your current shell session:

  bash:   source <(dots completion bash)
  zsh:    source <(dots completion zsh)
  fish:   dots completion fish | source

To install completions permanently, see your shell's documentation for
the appropriate completions directory (e.g. /etc/bash_completion.d/ or
~/.zsh/completions/).`,
		Annotations:       map[string]string{"skipConfig": "true"},
		ValidArgs:         []string{"bash", "zsh", "fish", "powershell"},
		Args:              cobra.ExactArgs(1),
		DisableFlagParsing: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := cmd.Root()
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(os.Stdout)
			case "zsh":
				return root.GenZshCompletion(os.Stdout)
			case "fish":
				return root.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return &errs.DotsError{
					Msg:  "unknown shell: " + args[0],
					Hint: "Valid shells: bash, zsh, fish, powershell",
				}
			}
		},
	}
	return cmd
}
