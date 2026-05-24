// Package shell manages the shell.d snippet directory and the marker-delimited
// source line inserted into ~/.zshrc / ~/.bashrc.
package shell

import "github.com/pkuehne/dots/internal/config"

// GenerateEnvSnippet returns the content of 010-env.sh from cfg.Env.
func GenerateEnvSnippet(cfg config.Config) string {
	panic("GenerateEnvSnippet: not yet implemented")
}

// GeneratePathSnippet returns the content of 020-path.sh from cfg.Shell.Path
// and per-tool path contributions.
func GeneratePathSnippet(cfg config.Config) string {
	panic("GeneratePathSnippet: not yet implemented")
}

// GenerateToolSnippet returns the content of 050-{name}.sh for one tool.
func GenerateToolSnippet(tool config.Tool) string {
	panic("GenerateToolSnippet: not yet implemented")
}

// WriteSnippets writes all generated snippets to cfg.Shell.Dir.
func WriteSnippets(cfg config.Config, dryRun bool) error {
	panic("WriteSnippets: not yet implemented")
}

// InsertSourceLine inserts the marker-delimited source line into the rc file.
func InsertSourceLine(cfg config.Config, dryRun bool) error {
	panic("InsertSourceLine: not yet implemented")
}

// Assembled returns the fully concatenated shell.d content in snippet order.
func Assembled(cfg config.Config) (string, error) {
	panic("Assembled: not yet implemented")
}

// Clean removes stale snippets from shell.d that are no longer in the config.
func Clean(cfg config.Config, dryRun bool) error {
	panic("Clean: not yet implemented")
}
