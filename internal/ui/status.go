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

// LabelWidth is the fixed width of the status-line label column, sized to the
// longest label dots emits ("backed up & wrote", 17 chars) so every row's name
// column stays aligned. Other aligned output (e.g. error rows) shares it.
const LabelWidth = 17

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
	case action == "updated" || action == "update":
		// An existing managed block refreshed in place (git/ssh include lines).
		return actionStyle{"✓", "updated", Green, "update"}
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

// FormatStatus renders one aligned, coloured status row — an icon, a
// fixed-width state label and the target name — without a trailing newline. In
// dry-run the past-tense label becomes "would <verb>" for mutating actions.
func FormatStatus(action, name string, dryRun bool) string {
	st := styleFor(action)
	label := st.label
	if dryRun && st.verb != "" {
		label = "would " + st.verb
	}
	return fmt.Sprintf("  %s %s  %s",
		Colorize(st.color, st.icon),
		Colorize(st.color, fmt.Sprintf("%-*s", LabelWidth, label)),
		name)
}

// StatusLine prints a single status row. Files, tools, repos and shell render
// through Section instead so apply groups each subsystem under a header; this
// remains for the few callers that print a lone row outside a section.
func StatusLine(action, name string, dryRun bool) {
	fmt.Println(FormatStatus(action, name, dryRun))
}

// sectionPrinted records whether any section header has been emitted in this
// process so the blank-line separator falls *between* sections rather than
// before the first one — apply starts directly with "Files:", not a blank line.
// A command renders its output once and exits, so this run-scoped flag never
// needs resetting.
var sectionPrinted bool

// Section groups status rows under a header so apply output reads as one block
// per subsystem (Files, Shell, Git, SSH, Repos, Tools) instead of an unlabelled
// run of lines. The header is printed at most once: always-listed sections call
// Header up front, while conditional sections (git/ssh/shell, silent when
// nothing changed) let the first Status/Summary print it lazily. A nil *Section
// is valid and prints rows with no header — used by tests that want bare output.
type Section struct {
	title   string
	started bool
}

// NewSection returns a Section that prints rows under "<title>:".
func NewSection(title string) *Section { return &Section{title: title} }

// Header prints the section title, once. Safe to call repeatedly and on a nil
// Section (which prints nothing).
func (s *Section) Header() {
	if s == nil || s.started {
		return
	}
	if sectionPrinted {
		// Blank line separating this section from the previous one.
		fmt.Println()
	}
	fmt.Printf("%s:\n", s.title)
	s.started = true
	sectionPrinted = true
}

// Status prints one status row under the section, emitting the header first if
// it has not been shown yet.
func (s *Section) Status(action, name string, dryRun bool) {
	s.Header()
	fmt.Println(FormatStatus(action, name, dryRun))
}

// Summary prints a tally line (e.g. "0 linked, 21 unchanged") indented to align
// under the section's rows. It ensures the header is shown so a section that is
// only ever summarised still gets its title.
func (s *Section) Summary(text string) {
	s.Header()
	fmt.Printf("  %s\n", text)
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
