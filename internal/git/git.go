// Package git manages the dots-owned managed.gitconfig and the [include] line
// inserted into ~/.gitconfig.
package git

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/pkuehne/dots/internal/config"
	"github.com/pkuehne/dots/internal/fileutil"
	"github.com/pkuehne/dots/internal/shell"
	"github.com/pkuehne/dots/internal/ui"
)

var gitIncludeBlock = shell.MarkerStart + "\n" +
	"[include]\n" +
	"    path = ~/.config/dots/git/managed.gitconfig\n" +
	shell.MarkerEnd

// GenerateConfig returns the content of managed.gitconfig from cfg.
func GenerateConfig(cfg config.Config) string {
	lines := []string{
		shell.GeneratedHeader,
		"# Source: dots.toml [git] + [[tool]] contributions",
		"# Regenerate: dots apply",
		"",
	}

	if cfg.Git.Name != "" || cfg.Git.Email != "" {
		lines = append(lines, "[user]")
		if cfg.Git.Name != "" {
			lines = append(lines, "    name = "+cfg.Git.Name)
		}
		if cfg.Git.Email != "" {
			lines = append(lines, "    email = "+cfg.Git.Email)
		}
		if cfg.Git.SigningKey != "" {
			lines = append(lines, "    signingkey = "+cfg.Git.SigningKey)
		}
		lines = append(lines, "")
	}

	var coreLines []string
	if cfg.Git.Editor != "" {
		coreLines = append(coreLines, "    editor = "+cfg.Git.Editor)
	}
	for _, tool := range cfg.Tools {
		if tool.Name == "delta" && tool.Git.Pager {
			if _, err := exec.LookPath("delta"); err == nil {
				coreLines = append(coreLines, "    pager = delta")
			}
		}
	}
	if len(coreLines) > 0 {
		lines = append(lines, "[core]")
		lines = append(lines, coreLines...)
		lines = append(lines, "")
	}

	lines = append(lines, "[init]")
	lines = append(lines, "    defaultBranch = "+cfg.Git.DefaultBranch)
	lines = append(lines, "")

	lines = append(lines, "[pull]")
	pullRebase := "false"
	if cfg.Git.PullRebase {
		pullRebase = "true"
	}
	lines = append(lines, "    rebase = "+pullRebase)
	lines = append(lines, "")

	if cfg.Git.Sign {
		lines = append(lines, "[commit]")
		lines = append(lines, "    gpgsign = true")
		lines = append(lines, "")
	}

	for _, tool := range cfg.Tools {
		if tool.Name == "delta" && tool.Git.Diff {
			if _, err := exec.LookPath("delta"); err == nil {
				lines = append(lines, "[diff]")
				lines = append(lines, "    tool = delta")
				lines = append(lines, "")
				lines = append(lines, `[difftool "delta"]`)
				lines = append(lines, `    cmd = delta "$LOCAL" "$REMOTE"`)
				lines = append(lines, "")
			}
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

// WriteManaged writes managed.gitconfig and inserts the [include] block into
// ~/.gitconfig. Both steps are idempotent. Like the file, repo and tool
// sections, it always renders its header, one row per managed item (including
// unchanged ones, unless summary suppresses them) and a tally, so the section
// stays visible on an idempotent re-run instead of vanishing. sec may be nil,
// in which case rows print without a section header.
func WriteManaged(cfg config.Config, dryRun, summary bool, sec *ui.Section) error {
	sec.Header()
	wrote, unchanged := 0, 0
	row := func(action, name string, changed bool) {
		if changed {
			wrote++
		} else {
			unchanged++
			action = "unchanged"
		}
		if !summary {
			sec.Status(action, name, dryRun)
		}
	}

	outPath := fileutil.Expand("~/.config/dots/git/managed.gitconfig")
	changed, err := fileutil.WriteIfChanged(outPath, []byte(GenerateConfig(cfg)), 0o644, dryRun)
	if err != nil {
		return err
	}
	row("wrote", outPath, changed)

	gitconfig := fileutil.Expand("~/.gitconfig")
	inserted, err := shell.InsertBlock(gitconfig, gitIncludeBlock, dryRun)
	if err != nil {
		return err
	}
	row("updated", "[include] → "+gitconfig, inserted)

	sec.Summary(ui.ChangeTally(wrote, unchanged, dryRun))
	return nil
}

// ShowManaged prints the would-be managed.gitconfig to stdout.
func ShowManaged(cfg config.Config) error {
	fmt.Print(GenerateConfig(cfg))
	return nil
}

// Uninit removes the marker-delimited [include] block from ~/.gitconfig.
func Uninit(cfg config.Config, dryRun bool) error {
	gitconfig := fileutil.Expand("~/.gitconfig")
	_, err := shell.RemoveBlock(gitconfig, dryRun)
	return err
}
