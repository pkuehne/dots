// Package ssh manages the dots-owned SSH config fragment and the Include line
// inserted into ~/.ssh/config.
package ssh

import "github.com/pkuehne/dots/internal/config"

// GenerateConfig returns the SSH config block for all active hosts.
func GenerateConfig(cfg config.Config, platform string) string {
	panic("GenerateConfig: not yet implemented")
}

// WriteManaged writes the SSH config fragment and inserts the Include line.
func WriteManaged(cfg config.Config, platform string, dryRun bool) error {
	panic("WriteManaged: not yet implemented")
}

// ShowManaged prints the would-be SSH config fragment to stdout.
func ShowManaged(cfg config.Config, platform string) error {
	panic("ShowManaged: not yet implemented")
}

// Uninit removes the marker-delimited Include line from ~/.ssh/config.
func Uninit(cfg config.Config, dryRun bool) error {
	panic("Uninit: not yet implemented")
}
