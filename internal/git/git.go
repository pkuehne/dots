// Package git manages the dots-owned managed.gitconfig and the [include] line
// inserted into ~/.gitconfig.
package git

import "github.com/pkuehne/dots/internal/config"

// GenerateConfig returns the content of managed.gitconfig from cfg.
func GenerateConfig(cfg config.Config) string {
	panic("GenerateConfig: not yet implemented")
}

// WriteManaged writes managed.gitconfig and inserts the [include] line.
func WriteManaged(cfg config.Config, dryRun bool) error {
	panic("WriteManaged: not yet implemented")
}

// ShowManaged prints the would-be managed.gitconfig to stdout.
func ShowManaged(cfg config.Config) error {
	panic("ShowManaged: not yet implemented")
}

// Uninit removes the marker-delimited [include] line from ~/.gitconfig.
func Uninit(cfg config.Config, dryRun bool) error {
	panic("Uninit: not yet implemented")
}
