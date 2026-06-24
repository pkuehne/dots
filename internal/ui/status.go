// Package ui renders the aligned, coloured status lines shared by apply output
// across subsystems (files, tools, repos, shell snippets, …). Keeping it in one
// place means every subsystem speaks the same visual vocabulary instead of each
// printing its own ad-hoc text.
package ui

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

// color auto-disables ANSI when stdout is not a terminal and honours the
// NO_COLOR convention, so piped or captured output stays plain text. The
// package-level colours below are reused across status output.
var (
	Green  = color.New(color.FgGreen)
	Yellow = color.New(color.FgYellow)
	Red    = color.New(color.FgRed)
	Cyan   = color.New(color.FgCyan)
	Dim    = color.New(color.FgHiBlack)
)

// actionStyle is the icon, label and colour used to render a deploy action.
type actionStyle struct {
	icon  string
	label string // past-tense state, used when the action has been applied
	color *color.Color
	// verb is the present-tense form used for the dry-run "would <verb>" label.
	// Empty for non-mutating actions (unchanged, skipped, missing).
	verb string
}

// styleFor maps an action to its presentation. It accepts both the applied
// verbs ("linked") and the dry-run/predicted forms ("link").
func styleFor(action string) actionStyle {
	switch {
	case action == "linked" || action == "link":
		return actionStyle{"✓", "linked", Green, "link"}
	case action == "copied" || action == "copy":
		return actionStyle{"✓", "copied", Green, "copy"}
	case action == "decrypted" || action == "decrypt":
		return actionStyle{"🔒", "decrypted", Cyan, "decrypt"}
	case action == "installed" || action == "install":
		return actionStyle{"✓", "installed", Green, "install"}
	case action == "cloned" || action == "clone":
		return actionStyle{"✓", "cloned", Green, "clone"}
	case action == "wrote" || action == "write":
		// Shell snippets and the rc bootstrapper block.
		return actionStyle{"✓", "wrote", Green, "write"}
	case action == "backed up & wrote":
		// A user file replaced after backing up the original (login shell rc).
		return actionStyle{"✓", action, Green, ""}
	case action == "removed" || action == "remove":
		// Stale snippet cleanup: a deliberate deletion, shown in yellow.
		return actionStyle{"−", "removed", Yellow, "remove"}
	case action == "present":
		// A tool already on PATH or a repo already cloned: nothing to do, shown
		// dimmed like "unchanged" so apply still lists it.
		return actionStyle{"·", "present", Dim, ""}
	case action == "diff":
		return actionStyle{"~", "drifted", Yellow, "update"}
	case action == "unchanged":
		return actionStyle{"·", "unchanged", Dim, ""}
	case action == "missing":
		return actionStyle{"⚠", "missing", Yellow, ""}
	case strings.HasPrefix(action, "skipped"):
		// Preserve the full reason if one is attached (e.g. profile/platform).
		return actionStyle{"⊘", action, Dim, ""}
	default:
		return actionStyle{"•", action, nil, ""}
	}
}

// StatusLine renders one aligned, coloured status row: an icon, a fixed-width
// state label and the target name. In dry-run the past-tense label becomes
// "would <verb>" for mutating actions. Files, tools, repos and shell all use
// this so apply output stays uniform across subsystems.
func StatusLine(action, name string, dryRun bool) {
	st := styleFor(action)
	label := st.label
	if dryRun && st.verb != "" {
		label = "would " + st.verb
	}
	fmt.Printf("  %s %s  %s\n",
		Colorize(st.color, st.icon),
		Colorize(st.color, fmt.Sprintf("%-16s", label)),
		name)
}

// Warn prints a yellow, indented warning line (no fixed-width label column),
// matching the visual weight of a status row's icon.
func Warn(format string, args ...any) {
	fmt.Printf("  %s %s\n", Colorize(Yellow, "⚠"), fmt.Sprintf(format, args...))
}

// Note prints a dimmed, indented informational line (e.g. "nothing to do").
func Note(s string) {
	fmt.Printf("  %s %s\n", Colorize(Dim, "·"), s)
}

// Colorize renders s in c, or returns it unstyled when c is nil. color handles
// the enabled/disabled decision internally.
func Colorize(c *color.Color, s string) string {
	if c == nil {
		return s
	}
	return c.Sprint(s)
}
