// Package deploy deploys a FileEntry to its destination: symlink, copy,
// template render, or age-decrypt as appropriate.
package deploy

import "github.com/pkuehne/dots/internal/config"

// Options controls deploy behaviour.
type Options struct {
	DryRun    bool
	ForceCopy bool // override symlink with copy
	Vars      map[string]any
}

// Result describes what happened when a single file was deployed.
type Result struct {
	Entry  config.FileEntry
	Action string // "linked", "copied", "skipped", "unchanged", "decrypted", "rendered"
	Err    error
}

// Apply deploys a single FileEntry and returns the result.
func Apply(entry config.FileEntry, opts Options) Result {
	panic("Apply: not yet implemented")
}

// ApplyAll deploys all entries and returns one Result per entry.
func ApplyAll(entries []config.FileEntry, opts Options) []Result {
	panic("ApplyAll: not yet implemented")
}

// Status returns the current deployment state of an entry without modifying anything.
func Status(entry config.FileEntry, repoRoot string) Result {
	panic("Status: not yet implemented")
}
