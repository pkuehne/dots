// Package repos clones and updates [[repo]] entries.
package repos

import "github.com/pkuehne/dots/internal/config"

// RepoState is the current state of a managed repo clone.
type RepoState struct {
	Entry   config.RepoEntry
	Exists  bool
	Dirty   bool
	Behind  int // commits behind remote
	Current string // current ref (branch or SHA)
}

// Clone clones any repos that are not yet present on disk.
// If names is non-empty, only those repos are cloned.
func Clone(cfg config.Config, names []string, dryRun bool) error {
	panic("Clone: not yet implemented")
}

// Update fetches and pulls repos that already exist.
func Update(cfg config.Config, names []string, dryRun bool) error {
	panic("Update: not yet implemented")
}

// Status returns the state of all managed repo clones.
func Status(cfg config.Config) ([]RepoState, error) {
	panic("Status: not yet implemented")
}

// Filter returns the subset of repos matching names. If names is empty, all repos are returned.
func Filter(repos []config.RepoEntry, names []string) []config.RepoEntry {
	panic("Filter: not yet implemented")
}
