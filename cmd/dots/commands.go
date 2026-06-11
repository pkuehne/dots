package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/pkuehne/dots/internal/deploy"
	"github.com/pkuehne/dots/internal/discovery"
	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/fileutil"
	"github.com/pkuehne/dots/internal/platform"
	"github.com/pkuehne/dots/internal/secrets"
	"github.com/pkuehne/dots/internal/shell"
	gossh "github.com/pkuehne/dots/internal/ssh"
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
			opts := deploy.Options{
				DryRun:        dryRun,
				ForceCopy:     forceCopy,
				RepoRoot:      globals.cfg.RepoRoot,
				DefaultMode:   globals.cfg.Meta.DefaultMode,
				ActiveProfile: globals.cfg.ActiveProfile,
			}

			entries, err := discovery.Walk(globals.cfg, platform.Detect())
			if err != nil {
				return err
			}

			results := deploy.ApplyAll(entries, opts)
			printResults(results, dryRun)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "print actions without executing")
	cmd.Flags().BoolVarP(&forceCopy, "copy", "c", false, "force copy mode instead of symlink")
	return cmd
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
			return todo("preview")
		},
	}
}

// ── status / diff / list ─────────────────────────────────────────────────────

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show deployment state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return todo("status")
		},
	}
}

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff [file]",
		Short: "Show diffs between source and deployed files",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return todo("diff")
		},
	}
}

func newListCmd() *cobra.Command {
	var showAll bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List managed files",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = showAll
			return todo("list")
		},
	}
	cmd.Flags().BoolVar(&showAll, "all", false, "include skipped files")
	return cmd
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

	var tag string

	check := &cobra.Command{
		Use:   "check [names...]",
		Short: "Check which configured tools are installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = tag
			return todo("tools check")
		},
	}
	check.Flags().StringVar(&tag, "tag", "", "filter by tag")

	install := &cobra.Command{
		Use:   "install [names...]",
		Short: "Install missing tools",
	}
	var dryRun, force bool
	install.Flags().StringVar(&tag, "tag", "", "filter by tag")
	install.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "print actions without executing")
	install.Flags().BoolVarP(&force, "force", "f", false, "reinstall even if present")
	install.RunE = func(cmd *cobra.Command, args []string) error {
		_ = tag
		_ = dryRun
		_ = force
		return todo("tools install")
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List configured tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = tag
			return todo("tools list")
		},
	}
	list.Flags().StringVar(&tag, "tag", "", "filter by tag")

	cmd.AddCommand(check, install, list)
	return cmd
}

// ── shell ────────────────────────────────────────────────────────────────────

func newShellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Manage shell integration",
	}

	show := &cobra.Command{
		Use:   "show",
		Short: "Print generated shell snippets",
	}
	var assembled bool
	show.Flags().BoolVar(&assembled, "assembled", false, "print full assembled output")
	show.RunE = func(cmd *cobra.Command, args []string) error {
		_ = assembled
		return todo("shell show")
	}

	clean := &cobra.Command{
		Use:   "clean",
		Short: "Remove stale snippets",
	}
	var dryRun bool
	clean.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "print actions without executing")
	clean.RunE = func(cmd *cobra.Command, args []string) error {
		_ = dryRun
		return todo("shell clean")
	}

	cmd.AddCommand(show, clean)
	return cmd
}

// ── repos ────────────────────────────────────────────────────────────────────

func newReposCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "repos", Short: "Manage git repositories"}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "clone [names...]",
			Short: "Clone missing repos",
			RunE:  func(cmd *cobra.Command, args []string) error { return todo("repos clone") },
		},
		&cobra.Command{
			Use:   "update [names...]",
			Short: "Update cloned repos",
			RunE:  func(cmd *cobra.Command, args []string) error { return todo("repos update") },
		},
		&cobra.Command{
			Use:   "status",
			Short: "Show repo states",
			RunE:  func(cmd *cobra.Command, args []string) error { return todo("repos status") },
		},
	)
	return cmd
}

// ── git ──────────────────────────────────────────────────────────────────────

func newGitCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "git", Short: "Manage git config"}

	initSub := &cobra.Command{
		Use:   "init",
		Short: "Enable git managed mode",
	}
	var dryRun bool
	initSub.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "print actions without executing")
	initSub.RunE = func(cmd *cobra.Command, args []string) error {
		_ = dryRun
		return todo("git init")
	}

	cmd.AddCommand(
		initSub,
		&cobra.Command{
			Use:   "show",
			Short: "Print managed.gitconfig",
			RunE:  func(cmd *cobra.Command, args []string) error { return todo("git show") },
		},
		&cobra.Command{
			Use:   "uninit",
			Short: "Remove dots [include] from ~/.gitconfig",
			RunE:  func(cmd *cobra.Command, args []string) error { return todo("git uninit") },
		},
	)
	return cmd
}

// ── ssh ──────────────────────────────────────────────────────────────────────

func newSSHCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "ssh", Short: "Manage SSH config"}

	initSub := &cobra.Command{Use: "init", Short: "Enable SSH managed mode"}
	var dryRun bool
	initSub.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "print actions without executing")
	initSub.RunE = func(cmd *cobra.Command, args []string) error {
		_ = dryRun
		return todo("ssh init")
	}

	cmd.AddCommand(
		initSub,
		&cobra.Command{
			Use:   "show",
			Short: "Print SSH config fragment",
			RunE:  func(cmd *cobra.Command, args []string) error { return todo("ssh show") },
		},
		&cobra.Command{
			Use:   "uninit",
			Short: "Remove dots Include from ~/.ssh/config",
			RunE:  func(cmd *cobra.Command, args []string) error { return todo("ssh uninit") },
		},
	)
	return cmd
}

// ── env ──────────────────────────────────────────────────────────────────────

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "env", Short: "Manage environment variables"}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "show",
			Short: "Print 010-env.sh content",
			RunE:  func(cmd *cobra.Command, args []string) error { return todo("env show") },
		},
		&cobra.Command{
			Use:   "check",
			Short: "Check [[env.when]] conditions",
			RunE:  func(cmd *cobra.Command, args []string) error { return todo("env check") },
		},
	)
	return cmd
}

// ── presets ──────────────────────────────────────────────────────────────────

func newPresetsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "presets", Short: "Manage presets"}

	show := &cobra.Command{
		Use:   "show <preset>",
		Short: "Print preset output",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return todo("presets show") },
	}

	eject := &cobra.Command{
		Use:   "eject <preset>",
		Short: "Eject preset to plain files",
		Args:  cobra.ExactArgs(1),
	}
	var dest string
	eject.Flags().StringVar(&dest, "dest", "", "output directory")
	eject.RunE = func(cmd *cobra.Command, args []string) error {
		_ = dest
		return todo("presets eject")
	}

	cmd.AddCommand(show, eject)
	return cmd
}
