package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/errs"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		if de, ok := errs.Unwrap(err); ok {
			fmt.Fprintln(os.Stderr, de.Render())
		} else {
			fmt.Fprintf(os.Stderr, "\n✗ %v\n", err)
		}
		os.Exit(1)
	}
}

// globals holds flags parsed by the root command and the loaded config, shared
// across all subcommands via pointer.
var globals = struct {
	profile string
	repo    string
	cfg     config.Config
}{}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "dots",
		Short:        "Dotfile management, tool installation, and shell environment generation",
		SilenceUsage: true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Annotations["skipConfig"] == "true" {
				return nil
			}
			repoRoot, err := config.FindRepoRoot(globals.repo)
			if err != nil {
				return err
			}
			cfg, err := config.Load(repoRoot, globals.profile)
			if err != nil {
				return err
			}
			globals.cfg = cfg
			return nil
		},
	}

	root.PersistentFlags().StringVar(&globals.profile, "profile", "", "activate a named profile")
	root.PersistentFlags().StringVar(&globals.repo, "repo", "", "path to dotfiles repository root")

	root.AddCommand(
		newInitCmd(),
		newApplyCmd(),
		newPreviewCmd(),
		newStatusCmd(),
		newDiffCmd(),
		newEditCmd(),
		newAddCmd(),
		newListCmd(),
		newDoctorCmd(),
		newMigrateCmd(),
		newEncryptCmd(),
		newDecryptCmd(),
		newToolsCmd(),
		newShellCmd(),
		newReposCmd(),
		newGitCmd(),
		newSSHCmd(),
		newEnvCmd(),
		newPresetsCmd(),
		newCompletionCmd(),
	)

	return root
}
