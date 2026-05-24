// Package presets generates opinionated config snippets for named presets (fzf, tmux).
// Presets are always off by default and can be ejected to plain files.
package presets

import "github.com/pkuehne/dots/internal/config"

// Generate returns the generated content for a named preset.
func Generate(name string, cfg config.Config) (string, error) {
	panic("Generate: not yet implemented")
}

// Eject writes a preset's generated content to dest as a plain file.
// After ejection the preset flag should be removed from dots.toml.
func Eject(name, dest string, cfg config.Config) error {
	panic("Eject: not yet implemented")
}

// Available returns the list of known preset names.
func Available() []string {
	return []string{"fzf", "tmux"}
}
