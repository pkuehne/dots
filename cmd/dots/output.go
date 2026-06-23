package main

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

// color auto-disables ANSI when stdout is not a terminal and honours the
// NO_COLOR convention, so piped or captured output stays plain text. The
// package-level colours below are reused across status output.
var (
	cGreen  = color.New(color.FgGreen)
	cYellow = color.New(color.FgYellow)
	cRed    = color.New(color.FgRed)
	cCyan   = color.New(color.FgCyan)
	cDim    = color.New(color.FgHiBlack)
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

// styleFor maps a deploy.Result.Action to its presentation. It accepts both the
// applied verbs ("linked") and the dry-run/predicted forms ("link").
func styleFor(action string) actionStyle {
	switch {
	case action == "linked" || action == "link":
		return actionStyle{"✓", "linked", cGreen, "link"}
	case action == "copied" || action == "copy":
		return actionStyle{"✓", "copied", cGreen, "copy"}
	case action == "decrypted" || action == "decrypt":
		return actionStyle{"🔒", "decrypted", cCyan, "decrypt"}
	case action == "installed" || action == "install":
		return actionStyle{"✓", "installed", cGreen, "install"}
	case action == "cloned" || action == "clone":
		return actionStyle{"✓", "cloned", cGreen, "clone"}
	case action == "present":
		// A tool already on PATH or a repo already cloned: nothing to do, shown
		// dimmed like "unchanged" so apply still lists it.
		return actionStyle{"·", "present", cDim, ""}
	case action == "diff":
		return actionStyle{"~", "drifted", cYellow, "update"}
	case action == "unchanged":
		return actionStyle{"·", "unchanged", cDim, ""}
	case action == "missing":
		return actionStyle{"⚠", "missing", cYellow, ""}
	case strings.HasPrefix(action, "skipped"):
		// Preserve the full reason if one is attached (e.g. profile/platform).
		return actionStyle{"⊘", action, cDim, ""}
	default:
		return actionStyle{"•", action, nil, ""}
	}
}

// printStatusLine renders one aligned, coloured status row: an icon, a
// fixed-width state label and the target name. In dry-run the past-tense label
// becomes "would <verb>" for mutating actions. Files, tools and repos all use
// this so apply output stays uniform across subsystems.
func printStatusLine(action, name string, dryRun bool) {
	st := styleFor(action)
	label := st.label
	if dryRun && st.verb != "" {
		label = "would " + st.verb
	}
	fmt.Printf("  %s %s  %s\n",
		colorize(st.color, st.icon),
		colorize(st.color, fmt.Sprintf("%-16s", label)),
		name)
}

// colorize renders s in c, or returns it unstyled when c is nil. color handles
// the enabled/disabled decision internally.
func colorize(c *color.Color, s string) string {
	if c == nil {
		return s
	}
	return c.Sprint(s)
}
