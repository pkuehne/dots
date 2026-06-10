package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pkuehne/dots/internal/deploy"
	"github.com/pkuehne/dots/internal/discovery"
	"github.com/pkuehne/dots/internal/errs"
	"github.com/pkuehne/dots/internal/platform"
	"github.com/pkuehne/dots/internal/secrets"
)

// ── init ─────────────────────────────────────────────────────────────────────

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold a new dots repository",
		Args:  cobra.MaximumNArgs(1),
		Annotations: map[string]string{"skipConfig": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return todo("init")
		},
	}
	return cmd
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
			return todo("edit")
		},
	}
}

func newAddCmd() *cobra.Command {
	var dest string
	cmd := &cobra.Command{
		Use:   "add <path>",
		Short: "Adopt an existing file into the repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = dest
			return todo("add")
		},
	}
	cmd.Flags().StringVar(&dest, "dest", "", "override repo destination path")
	return cmd
}

// ── doctor / migrate ─────────────────────────────────────────────────────────

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "System health check",
		RunE: func(cmd *cobra.Command, args []string) error {
			return todo("doctor")
		},
	}
}

func newMigrateCmd() *cobra.Command {
	var write bool
	var platform string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Scan for unmanaged dotfiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = write
			_ = platform
			return todo("migrate")
		},
	}
	cmd.Flags().BoolVar(&write, "write", false, "copy files and add entries to dots.toml")
	cmd.Flags().StringVar(&platform, "platform", "", "target platform directory")
	return cmd
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
