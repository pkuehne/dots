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
	"github.com/pkuehne/dots/internal/lockfile"
	"github.com/pkuehne/dots/internal/platform"
	"github.com/pkuehne/dots/internal/presets"
	"github.com/pkuehne/dots/internal/repos"
	"github.com/pkuehne/dots/internal/secrets"
	"github.com/pkuehne/dots/internal/selfupdate"
	"github.com/pkuehne/dots/internal/shell"
	gossh "github.com/pkuehne/dots/internal/ssh"
	"github.com/pkuehne/dots/internal/tools"
	"github.com/pkuehne/dots/internal/ui"
)

// ── init ─────────────────────────────────────────────────────────────────────

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:         "init [dir]",
		Short:       "Scaffold a new dots repository",
		Args:        cobra.MaximumNArgs(1),
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

// ── upgrade ──────────────────────────────────────────────────────────────────

func newUpgradeCmd() *cobra.Command {
	var (
		dryRun     bool
		force      bool
		checkOnly  bool
		pinVersion string
	)
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade the dots binary to the latest release",
		Long: "Download the latest dots release from GitHub and replace the running\n" +
			"binary in place. Use --version to pin a release, --check to only report\n" +
			"availability, or --dry-run to preview without writing.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"skipConfig": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade(selfupdate.Options{
				CurrentVersion: version,
				TargetVersion:  pinVersion,
				Force:          force,
				DryRun:         dryRun || checkOnly,
			}, checkOnly)
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would happen without writing")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "upgrade even if already current or a dev build")
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only report whether a newer version is available")
	cmd.Flags().StringVar(&pinVersion, "version", "", "pin a specific release tag instead of the latest")
	return cmd
}

func runUpgrade(opts selfupdate.Options, checkOnly bool) error {
	if checkOnly {
		res, err := selfupdate.Check(opts)
		if err != nil {
			return err
		}
		printUpgradeState(res)
		return nil
	}

	res, err := selfupdate.Run(opts)
	if err != nil {
		return err
	}

	switch {
	case res.Applied:
		printStatusLine("installed", fmt.Sprintf("dots %s → %s", res.CurrentVersion, res.LatestVersion), false)
		if res.BackupPath != "" {
			fmt.Printf("  %s\n", colorize(cDim, "previous binary saved to "+res.BackupPath))
		}
	case res.Available && opts.DryRun:
		printStatusLine("install", fmt.Sprintf("dots %s → %s", res.CurrentVersion, res.LatestVersion), true)
	default:
		printUpgradeState(res)
	}
	return nil
}

// printUpgradeState reports the current/latest comparison without implying an
// action was taken (used by --check and the already-current path).
func printUpgradeState(res selfupdate.Result) {
	switch {
	case res.Available:
		fmt.Printf("  %s dots %s is available (current: %s). Run 'dots upgrade'.\n",
			colorize(cYellow, "⚠"), res.LatestVersion, res.CurrentVersion)
	case res.CurrentVersion == "dev":
		fmt.Printf("  %s running a dev build; latest release is %s. Use --force to install it.\n",
			colorize(cDim, "·"), res.LatestVersion)
	default:
		fmt.Printf("  %s dots is up to date (%s).\n", colorize(cGreen, "✓"), res.CurrentVersion)
	}
}

// ── apply / preview ──────────────────────────────────────────────────────────

func newApplyCmd() *cobra.Command {
	var dryRun, forceCopy, summary bool

	cmd := &cobra.Command{
		Use:   "apply [files...]",
		Short: "Deploy files and generate managed configs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(globals.cfg, args, dryRun, forceCopy, summary)
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "print actions without executing")
	cmd.Flags().BoolVarP(&forceCopy, "copy", "c", false, "force copy mode instead of symlink")
	cmd.Flags().BoolVarP(&summary, "summary", "s", false, "print only the summary line, not per-file status")
	return cmd
}

func runApply(cfg config.Config, fileArgs []string, dryRun, forceCopy, summary bool) error {
	filesOnly := len(fileArgs) > 0

	opts := deploy.Options{
		DryRun:        dryRun,
		ForceCopy:     forceCopy,
		RepoRoot:      cfg.RepoRoot,
		DefaultMode:   cfg.Meta.DefaultMode,
		ActiveProfile: cfg.ActiveProfile,
		Platforms:     platform.Platforms(),
		Secrets:       cfg.Secrets,
	}

	entries, err := discovery.Walk(cfg, platform.Platforms())
	if err != nil {
		return err
	}

	if filesOnly {
		matched := make(map[string]bool, len(fileArgs))
		var filtered []config.FileEntry
		for _, e := range entries {
			added := false
			for _, a := range fileArgs {
				if strings.Contains(e.Src, a) || strings.Contains(e.Dst, a) ||
					filepath.Base(e.Dst) == a {
					matched[a] = true
					if !added {
						filtered = append(filtered, e)
						added = true
					}
				}
			}
		}
		// A name that matches nothing is almost always a typo; failing loudly
		// beats a misleading "0 linked, 0 copied" success.
		var unmatched []string
		for _, a := range fileArgs {
			if !matched[a] {
				unmatched = append(unmatched, a)
			}
		}
		if len(unmatched) > 0 {
			return errs.New(
				"no managed file matches: "+strings.Join(unmatched, ", "),
				"Run 'dots list' to see managed files, or check the name or path.",
			)
		}
		entries = filtered
	}

	results := deploy.ApplyAll(entries, opts)
	if n := printResults(results, dryRun, summary); n > 0 {
		return errs.New(
			fmt.Sprintf("%d file(s) failed to deploy", n),
			"See errors above.",
		)
	}

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
	if err := applyRepos(cfg, dryRun, summary); err != nil {
		return err
	}
	if err := applyPresets(cfg, dryRun); err != nil {
		return err
	}
	if err := applyTools(cfg, dryRun, summary); err != nil {
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
	sec := ui.NewSection("Shell")
	if err := shell.WriteSnippets(cfg, dryRun, sec); err != nil {
		return err
	}
	return shell.InsertSourceLine(cfg, dryRun, sec)
}

func applyGit(cfg config.Config, dryRun bool) error {
	if !cfg.Git.Managed {
		return nil
	}
	return gogit.WriteManaged(cfg, dryRun, ui.NewSection("Git"))
}

func applySSH(cfg config.Config, dryRun bool) error {
	if !cfg.SSH.Managed {
		return nil
	}
	return gossh.WriteManaged(cfg, platform.Platforms(), dryRun, ui.NewSection("SSH"))
}

func applyRepos(cfg config.Config, dryRun, summary bool) error {
	if len(cfg.Repos) == 0 {
		return nil
	}
	results, err := repos.Clone(cfg, nil, dryRun)
	printCloneResults(results, dryRun, summary)
	return err
}

// printCloneResults renders the Repos section: one coloured status row per repo
// (unless summary) under a header, followed by a tally. Listing every repo —
// including ones already cloned — keeps it consistent with the file and tool
// sections rather than only reporting the ones it touched.
func printCloneResults(results []repos.CloneResult, dryRun, summary bool) {
	sec := ui.NewSection("Repos")
	sec.Header()
	cloned, present := 0, 0
	for _, r := range results {
		switch r.Action {
		case "cloned":
			cloned++
		case "present":
			present++
		}
		if summary {
			continue
		}
		sec.Status(r.Action, r.Entry.Dst, dryRun)
	}
	verb := "cloned"
	if dryRun {
		verb = "to clone"
	}
	sec.Summary(fmt.Sprintf("%d %s, %d present", cloned, verb, present))
}

func applyPresets(cfg config.Config, dryRun bool) error {
	if cfg.Presets.Fzf && cfg.Shell.Managed {
		// fzf key-binding paths differ per shell, and a .sh snippet is sourced
		// by both bootstrappers, so write a shell-specific 030-fzf.<shell> for
		// each. shell.Clean's `expected` map must list both so apply and clean
		// stay in sync. These are dots-owned; writeUserFile skips the write when
		// unchanged and overwrites without backup (the content is generated).
		dir := fileutil.Expand(cfg.Shell.Dir)
		for _, shellName := range []string{"zsh", "bash"} {
			dst := filepath.Join(dir, "030-fzf."+shellName)
			content := presets.GenerateFzf(shellName)
			if dryRun {
				fmt.Printf("  would write fzf preset → %s\n", dst)
				continue
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			action, err := writeUserFile(dst, content)
			if err != nil {
				return err
			}
			fmt.Printf("  %s fzf preset → %s\n", action, dst)
		}
	}
	if cfg.Presets.Tmux {
		content, err := presets.Generate("tmux", cfg)
		if err != nil {
			return err
		}
		dst := fileutil.Expand("~/.tmux.conf")
		if dryRun {
			fmt.Printf("  would write tmux preset → %s\n", dst)
		} else {
			action, err := writeUserFile(dst, content)
			if err != nil {
				return err
			}
			fmt.Printf("  %s tmux preset → %s\n", action, dst)
		}
	}
	return nil
}

// isDotsGenerated reports whether content begins with a dots generated-file
// header — either the shell header or a preset header. Such files are owned by
// dots and may be overwritten without a backup.
func isDotsGenerated(content string) bool {
	return strings.HasPrefix(content, shell.GeneratedHeader) ||
		strings.HasPrefix(content, "# Generated by dots preset:")
}

// writeUserFile writes content to a user-owned file (one outside dots-owned
// shell.d) while honoring invariants 3 and 8: it skips the write when the
// content is already identical, and backs up any pre-existing file that is not
// dots-generated before overwriting it. It returns a verb describing what
// happened ("unchanged", "wrote", or "backed up & wrote").
func writeUserFile(dst, content string) (string, error) {
	existing, err := os.ReadFile(dst)
	switch {
	case err == nil:
		if string(existing) == content {
			return "unchanged", nil
		}
		if !isDotsGenerated(string(existing)) {
			if _, err := fileutil.Backup(dst); err != nil {
				return "", err
			}
			if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
				return "", err
			}
			return "backed up & wrote", nil
		}
	case !os.IsNotExist(err):
		return "", err
	}
	if err := fileutil.EnsureParent(dst); err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
		return "", err
	}
	return "wrote", nil
}

// checkKnownTools returns an error if any of names does not match a configured
// tool, so a typo fails loudly rather than silently matching nothing. A name
// that exists but is scoped out by tag/platform/profile is still "known".
func checkKnownTools(allTools []config.Tool, names []string) error {
	known := make(map[string]bool, len(allTools))
	for _, t := range allTools {
		known[t.Name] = true
	}
	var unknown []string
	for _, n := range names {
		if !known[n] {
			unknown = append(unknown, n)
		}
	}
	if len(unknown) > 0 {
		return errs.New(
			"unknown tool(s): "+strings.Join(unknown, ", "),
			"Run 'dots tools list' to see configured tools.",
		)
	}
	return nil
}

func applyTools(cfg config.Config, dryRun, summary bool) error {
	if len(cfg.Tools) == 0 {
		return nil
	}
	plat := platform.Detect()
	arch := platform.Arch()
	active := tools.Filter(cfg.Tools, nil, "", platform.Platforms(), cfg.ActiveProfile)
	results := tools.Check(active)
	lock, err := lockfile.Load("")
	if err != nil {
		return err
	}
	opts := tools.InstallOptions{DryRun: dryRun, Lock: lock}
	sec := ui.NewSection("Tools")
	sec.Header()
	installed, present, installErrors := 0, 0, 0
	for _, r := range results {
		// List every tool — already-present ones included — so apply reports the
		// full set rather than only the tools it touched (#26).
		if r.Installed {
			present++
			if !summary {
				sec.Status("present", r.Tool.Name, dryRun)
			}
			continue
		}
		if err := tools.Install(r.Tool, cfg, plat, arch, opts); err != nil {
			fmt.Fprintf(os.Stderr, "  %s %s  %s: %v\n",
				colorize(cRed, "✗"), colorize(cRed, fmt.Sprintf("%-*s", ui.LabelWidth, "error")), r.Tool.Name, err)
			installErrors++
		} else {
			installed++
			if !summary {
				sec.Status("installed", r.Tool.Name, dryRun)
			}
		}
	}
	if !dryRun {
		if err := lock.Save(); err != nil {
			return err
		}
	}
	verb := "installed"
	if dryRun {
		verb = "to install"
	}
	tally := fmt.Sprintf("%d %s, %d present", installed, verb, present)
	if installErrors > 0 {
		tally += fmt.Sprintf(", %d errors", installErrors)
	}
	sec.Summary(tally)
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
	sec := ui.NewSection("Login shell")
	zprofile := fileutil.Expand("~/.zprofile")
	profile := fileutil.Expand("~/.profile")
	if dryRun {
		sec.Status("wrote", zprofile, true)
		sec.Status("wrote", profile, true)
		return nil
	}
	zAction, err := writeUserFile(zprofile, presets.GenerateZprofile(cfg))
	if err != nil {
		return err
	}
	pAction, err := writeUserFile(profile, presets.GenerateProfile(cfg))
	if err != nil {
		return err
	}
	sec.Status(zAction, zprofile, false)
	sec.Status(pAction, profile, false)
	return nil
}

func printResults(results []deploy.Result, dryRun, summary bool) int {
	sec := ui.NewSection("Files")
	sec.Header()
	counts := map[string]int{}
	for _, r := range results {
		if r.Err != nil {
			// Errors always surface, even in summary mode.
			fmt.Fprintf(os.Stderr, "  %s  %s: %v\n",
				colorize(cRed, "✗ error"), r.Entry.Dst, r.Err)
			counts["error"]++
			continue
		}
		// All skip variants (plain, platform/profile) roll up into a single
		// "skipped" tally so the summary count is accurate.
		if strings.HasPrefix(r.Action, "skipped") {
			counts["skipped"]++
		} else {
			counts[r.Action]++
		}
		if summary {
			continue
		}
		sec.Status(r.Action, r.Entry.Dst, dryRun)
	}
	tally := fmt.Sprintf("%d linked, %d copied, %d unchanged, %d skipped",
		counts["linked"]+counts["link"],
		counts["copied"]+counts["copy"],
		counts["unchanged"],
		counts["skipped"],
	)
	if n := counts["decrypted"] + counts["decrypt"]; n > 0 {
		tally += fmt.Sprintf(", %d decrypted", n)
	}
	if counts["missing"] > 0 {
		tally += fmt.Sprintf(", %d missing", counts["missing"])
	}
	if counts["error"] > 0 {
		tally += fmt.Sprintf(", %d errors", counts["error"])
	}
	sec.Summary(tally)
	return counts["error"]
}

func newPreviewCmd() *cobra.Command {
	var summary bool
	cmd := &cobra.Command{
		Use:   "preview [files...]",
		Short: "Alias for apply --dry-run",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runApply(globals.cfg, args, true, false, summary)
		},
	}
	cmd.Flags().BoolVarP(&summary, "summary", "s", false, "print only the summary line, not per-file status")
	return cmd
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
	entries, err := discovery.Walk(cfg, platform.Platforms())
	if err != nil {
		return err
	}
	opts := deploy.Options{
		RepoRoot:      cfg.RepoRoot,
		DefaultMode:   cfg.Meta.DefaultMode,
		ActiveProfile: cfg.ActiveProfile,
		Platforms:     platform.Platforms(),
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
		case "secret":
			icon = "🔒"
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
	entries, err := discovery.Walk(cfg, platform.Platforms())
	if err != nil {
		return err
	}
	opts := deploy.Options{
		RepoRoot:      cfg.RepoRoot,
		DefaultMode:   cfg.Meta.DefaultMode,
		ActiveProfile: cfg.ActiveProfile,
		Platforms:     platform.Platforms(),
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
	entries, err := discovery.Walk(cfg, platform.Platforms())
	if err != nil {
		return err
	}
	opts := deploy.Options{
		RepoRoot:      cfg.RepoRoot,
		DefaultMode:   cfg.Meta.DefaultMode,
		ActiveProfile: cfg.ActiveProfile,
		Platforms:     platform.Platforms(),
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
	entries, err := discovery.Walk(globals.cfg, platform.Platforms())
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

	relSrc, err := filepath.Rel(repoRoot, repoDest)
	if err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	dstStr := src
	if strings.HasPrefix(dstStr, home) {
		dstStr = "~" + dstStr[len(home):]
	}

	// Idempotency (invariant 3): if this file is already managed — an existing
	// [[file]] entry shares its src or dst, or the repo destination already
	// exists — do nothing rather than appending a duplicate entry.
	for _, e := range globals.cfg.Files {
		if e.Src == relSrc || e.Dst == dstStr {
			fmt.Printf("  already managed: %s\n", relSrc)
			return nil
		}
	}
	if _, err := os.Stat(repoDest); err == nil {
		fmt.Printf("  already managed: %s\n", relSrc)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(repoDest), 0o755); err != nil {
		return err
	}
	if err := fileutil.CopyFile(src, repoDest); err != nil {
		return err
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

	// age secrets (if .age files present). age is linked into the binary, so
	// no external tool is needed — but decrypting .age files needs an identity.
	// filepath.Glob does not recurse, so walk the tree (skipping .git).
	var ageFiles []string
	_ = filepath.WalkDir(cfg.RepoRoot, func(p string, d os.DirEntry, _ error) error {
		if d != nil && d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if strings.HasSuffix(p, ".age") {
			ageFiles = append(ageFiles, p)
		}
		return nil
	})
	if len(ageFiles) > 0 {
		identity := fileutil.Expand(cfg.Secrets.Identity)
		_, identityErr := os.Stat(identity)
		switch {
		case cfg.Secrets.Identity == "":
			fail(".age files present but no [secrets] identity configured")
		case identityErr != nil:
			fail(fmt.Sprintf("age identity file not found: %s", identity))
		default:
			ok("age identity available (.age files decryptable)")
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

	// The configured tools bin_dir on PATH (this is where downloaded tools land,
	// not a hardcoded ~/.local/bin which may not match the user's config).
	binDir := cfg.ToolsConfig.BinDir
	if binDir == "" {
		binDir = "~/.local/bin"
	}
	binDirExpanded := fileutil.Expand(binDir)
	if strings.Contains(os.Getenv("PATH"), binDirExpanded) {
		ok(binDir + " on PATH")
	} else {
		warn(binDir + " not on PATH")
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
		// Bsize is int64 on Linux but uint32 on Darwin; cast both operands so the
		// multiplication compiles on every platform.
		freeMB := int64(st.Bavail) * int64(st.Bsize) / (1024 * 1024)
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
		// doctor is advisory: warnings alone do not fail the command.
		fmt.Printf("%d warning(s)\n", warnings)
		return 0
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
			if err := checkKnownTools(globals.cfg.Tools, args); err != nil {
				return err
			}
			filtered := tools.Filter(globals.cfg.Tools, args, checkTag, platform.Platforms(), globals.cfg.ActiveProfile)
			if len(filtered) == 0 {
				fmt.Println("  No tools configured.")
				return nil
			}
			results := tools.Check(filtered)
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
			if err := checkKnownTools(globals.cfg.Tools, args); err != nil {
				return err
			}
			plat := platform.Detect()
			arch := platform.Arch()
			filtered := tools.Filter(globals.cfg.Tools, args, installTag, platform.Platforms(), globals.cfg.ActiveProfile)
			if !installForce {
				results := tools.Check(filtered)
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
			lock, err := lockfile.Load("")
			if err != nil {
				return err
			}
			opts := tools.InstallOptions{DryRun: installDryRun, Force: installForce, Lock: lock}
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
			if !installDryRun {
				if err := lock.Save(); err != nil {
					return err
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

	var updateTag string
	var updateDryRun bool
	update := &cobra.Command{
		Use:   "update [names...]",
		Short: "Update version-tracked tools to their target version",
		Long: "Reinstall github-method tools whose installed version differs from\n" +
			"their target (a pinned `version`, or the latest release when version is\n" +
			"unset or \"latest\"). Tools installed via a package manager are left to\n" +
			"that manager.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkKnownTools(globals.cfg.Tools, args); err != nil {
				return err
			}
			plat := platform.Detect()
			arch := platform.Arch()
			lock, err := lockfile.Load("")
			if err != nil {
				return err
			}
			results, err := tools.Update(globals.cfg, args, updateTag, plat, arch, lock, updateDryRun)
			if err != nil {
				return err
			}
			if !updateDryRun {
				if err := lock.Save(); err != nil {
					return err
				}
			}
			printUpdateResults(results)
			return nil
		},
	}
	update.Flags().StringVar(&updateTag, "tag", "", "filter by tag")
	update.Flags().BoolVarP(&updateDryRun, "dry-run", "n", false, "print actions without executing")

	var statusTag string
	status := &cobra.Command{
		Use:   "status [names...]",
		Short: "Show installed vs target versions for tracked tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkKnownTools(globals.cfg.Tools, args); err != nil {
				return err
			}
			filtered := tools.Filter(globals.cfg.Tools, args, statusTag, platform.Platforms(), globals.cfg.ActiveProfile)
			if len(filtered) == 0 {
				fmt.Println("  No tools configured.")
				return nil
			}
			lock, err := lockfile.Load("")
			if err != nil {
				return err
			}
			printVersionStatus(tools.VersionStatus(filtered, platform.Detect(), lock))
			return nil
		},
	}
	status.Flags().StringVar(&statusTag, "tag", "", "filter by tag")

	var listTag string
	list := &cobra.Command{
		Use:   "list",
		Short: "List configured tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			filtered := tools.Filter(globals.cfg.Tools, args, listTag, platform.Platforms(), globals.cfg.ActiveProfile)
			if len(filtered) == 0 {
				fmt.Println("  No tools configured.")
				return nil
			}
			for _, t := range filtered {
				tags := ""
				if len(t.Tags) > 0 {
					tags = " [" + strings.Join(t.Tags, ", ") + "]"
				}
				desc := ""
				if t.Desc != "" {
					desc = "  " + t.Desc
				}
				fmt.Printf("  %-20s%s%s\n", t.Name, desc, tags)
			}
			return nil
		},
	}
	list.Flags().StringVar(&listTag, "tag", "", "filter by tag")

	cmd.AddCommand(check, install, update, status, list)
	return cmd
}

// printUpdateResults renders the outcome of `dots tools update`.
func printUpdateResults(results []tools.UpdateResult) {
	changed := 0
	for _, r := range results {
		switch r.Action {
		case "updated":
			printStatusLine("updated", fmt.Sprintf("%s %s → %s", r.Tool.Name, displayVer(r.From), r.To), false)
			changed++
		case "installed":
			printStatusLine("installed", fmt.Sprintf("%s %s", r.Tool.Name, r.To), false)
			changed++
		case "would-update":
			printStatusLine("update", fmt.Sprintf("%s %s → %s", r.Tool.Name, displayVer(r.From), r.To), true)
			changed++
		case "would-install":
			printStatusLine("install", fmt.Sprintf("%s %s", r.Tool.Name, r.To), true)
			changed++
		case "uptodate":
			printStatusLine("up-to-date", fmt.Sprintf("%s %s", r.Tool.Name, r.To), false)
		case "untracked":
			fmt.Printf("  %s %s  %s\n", colorize(cDim, "·"),
				colorize(cDim, fmt.Sprintf("%-*s", ui.LabelWidth, "untracked")), r.Tool.Name)
		}
	}
	if changed == 0 {
		fmt.Printf("\n%s All tracked tools are up to date.\n", colorize(cGreen, "✓"))
	}
}

// printVersionStatus renders the table for `dots tools status`.
func printVersionStatus(states []tools.VersionState) {
	for _, s := range states {
		switch s.State {
		case tools.UpToDate:
			fmt.Printf("  %s %-20s %s\n", colorize(cGreen, "✓"), s.Tool.Name, s.Target)
		case tools.Outdated:
			fmt.Printf("  %s %-20s %s → %s\n", colorize(cYellow, "↑"), s.Tool.Name, displayVer(s.Installed), s.Target)
		case tools.NotInstalled:
			fmt.Printf("  %s %-20s %s (not installed)\n", colorize(cRed, "✗"), s.Tool.Name, s.Target)
		case tools.NotTracked:
			fmt.Printf("  %s %-20s %s\n", colorize(cDim, "·"), s.Tool.Name, colorize(cDim, "untracked"))
		case tools.Unknown:
			fmt.Printf("  %s %-20s %s\n", colorize(cYellow, "?"), s.Tool.Name, colorize(cDim, "target unresolved"))
		}
	}
}

// displayVer renders a version for output, substituting a dash for the empty
// string (e.g. a tool tracked but never recorded in the lockfile).
func displayVer(v string) string {
	if v == "" {
		return "—"
	}
	return v
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
			results, err := repos.Clone(globals.cfg, args, cloneDryRun)
			printCloneResults(results, cloneDryRun, false)
			return err
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
				case s.Ref != "" && !s.OnTarget:
					icon, state = "≠", "ref "+s.Ref
				case s.Behind > 0:
					icon, state = "↓", fmt.Sprintf("behind %d", s.Behind)
				case s.Ref != "":
					state = "ref " + s.Ref
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
			return gogit.WriteManaged(globals.cfg, initDryRun, ui.NewSection("Git"))
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
			return gossh.WriteManaged(globals.cfg, platform.Platforms(), initDryRun, ui.NewSection("SSH"))
		},
	}
	initSub.Flags().BoolVarP(&initDryRun, "dry-run", "n", false, "print actions without executing")

	show := &cobra.Command{
		Use:   "show",
		Short: "Print SSH config fragment",
		RunE: func(cmd *cobra.Command, args []string) error {
			return gossh.ShowManaged(globals.cfg, platform.Platforms())
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
		Annotations: map[string]string{"skipConfig": "true"},
		ValidArgs:   []string{"bash", "zsh", "fish", "powershell"},
		Args:        cobra.ExactArgs(1),
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
				return errs.New("unknown shell: "+args[0],
					"Valid shells: bash, zsh, fish, powershell")
			}
		},
	}
	return cmd
}
