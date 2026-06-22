package main

import (
	"os"
	"strings"
)

// useColor reports whether stdout should be styled with ANSI escapes. It honours
// the de-facto NO_COLOR convention and TERM=dumb, and otherwise only colours when
// stdout is a terminal — so piped or captured output stays plain text.
var useColor = shouldColor()

func shouldColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ANSI SGR codes used for status output.
const (
	cReset  = "\x1b[0m"
	cGreen  = "\x1b[32m"
	cYellow = "\x1b[33m"
	cRed    = "\x1b[31m"
	cCyan   = "\x1b[36m"
	cDim    = "\x1b[90m"
)

// colorize wraps s in the given ANSI code when colour is enabled.
func colorize(code, s string) string {
	if !useColor || code == "" {
		return s
	}
	return code + s + cReset
}

// actionStyle is the icon, label and colour used to render a deploy action.
type actionStyle struct {
	icon  string
	label string // past-tense state, used when the action has been applied
	color string
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
		return actionStyle{"•", action, "", ""}
	}
}
