package main

import (
	"github.com/fatih/color"

	"github.com/pkuehne/dots/internal/ui"
)

// The status-line rendering lives in internal/ui so every subsystem (files,
// tools, repos, shell) speaks the same visual vocabulary. These aliases keep
// the short names the command layer reaches for.
var (
	cGreen  = ui.Green
	cYellow = ui.Yellow
	cRed    = ui.Red
	cDim    = ui.Dim
)

func printStatusLine(action, name string, dryRun bool) { ui.StatusLine(action, name, dryRun) }

func colorize(c *color.Color, s string) string { return ui.Colorize(c, s) }
