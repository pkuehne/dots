package main

import (
	"sort"

	"github.com/spf13/cobra"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/presets"
)

// This file wires dynamic shell completion onto the commands that take a name
// argument (tool, repo, preset, profile). Cobra emits these candidates — with
// their descriptions — into the shell's completion system, which fzf-tab then
// renders in its picker.
//
// Completion functions run under the hidden `__complete` command, which does
// not execute the root PersistentPreRunE, so globals.cfg is not populated.
// completionConfig loads it here, leniently: on any error we offer nothing
// (ShellCompDirectiveNoFileComp) rather than surfacing an error into the shell.

// completionConfig loads the config the same way the root PersistentPreRunE
// does, honouring --repo and --profile. ok is false if the repo or config
// cannot be resolved (e.g. running outside a dotfiles repo).
func completionConfig() (config.Config, bool) {
	repoRoot, err := config.FindRepoRoot(globals.repo)
	if err != nil {
		return config.Config{}, false
	}
	cfg, err := config.Load(repoRoot, globals.profile)
	if err != nil {
		return config.Config{}, false
	}
	return cfg, true
}

// contains reports whether s is already present in args (used to drop
// already-typed names from variadic [names...] completions).
func contains(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

// completeToolNames completes configured tool names, annotated with their
// description, skipping names already present on the command line.
func completeToolNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, ok := completionConfig()
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []string
	for _, t := range cfg.Tools {
		if contains(args, t.Name) {
			continue
		}
		if t.Desc != "" {
			out = append(out, t.Name+"\t"+t.Desc)
		} else {
			out = append(out, t.Name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeToolTags completes the distinct set of tags across configured tools.
func completeToolTags(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, ok := completionConfig()
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range cfg.Tools {
		for _, tag := range t.Tags {
			if !seen[tag] {
				seen[tag] = true
				out = append(out, tag)
			}
		}
	}
	sort.Strings(out)
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeRepoNames completes configured repo names, skipping names already
// present on the command line.
func completeRepoNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, ok := completionConfig()
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []string
	for _, r := range cfg.Repos {
		if !contains(args, r.Name) {
			out = append(out, r.Name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completePresetNames completes the known preset names. It needs no config, so
// it works anywhere.
func completePresetNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return presets.Available(), cobra.ShellCompDirectiveNoFileComp
}

// completeProfileNames completes the profile names declared in the [profiles]
// table of dots.toml. config.Load populates cfg.Profiles from the raw table.
func completeProfileNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cfg, ok := completionConfig()
	if !ok {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, cobra.ShellCompDirectiveNoFileComp
}
