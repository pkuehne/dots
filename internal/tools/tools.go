// Package tools checks and installs [[tool]] entries using the method declared
// in each [[tool.install]] block (apt, github, brew, script, …).
package tools

import "github.com/pkuehne/dots/internal/config"

// CheckResult is the outcome of checking whether one tool is present.
type CheckResult struct {
	Tool      config.Tool
	Installed bool
	Version   string // empty if not installed or version detection not supported
}

// InstallOptions controls install behaviour.
type InstallOptions struct {
	DryRun bool
	Force  bool // reinstall even if already present
}

// Check returns whether each tool is currently installed.
func Check(tools []config.Tool, platform, arch string) []CheckResult {
	panic("Check: not yet implemented")
}

// Install installs a single tool using the best matching [[tool.install]] entry
// for the current platform and architecture.
func Install(tool config.Tool, cfg config.Config, platform, arch string, opts InstallOptions) error {
	panic("Install: not yet implemented")
}

// Filter returns the subset of tools matching the given names or tag.
// If both are empty, all tools are returned.
func Filter(tools []config.Tool, names []string, tag string) []config.Tool {
	panic("Filter: not yet implemented")
}
