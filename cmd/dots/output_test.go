package main

import (
	"strings"
	"testing"

	"github.com/fatih/color"
)

func TestColorize_RespectsColorEnabled(t *testing.T) {
	orig := color.NoColor
	defer func() { color.NoColor = orig }()

	color.NoColor = true
	if got := colorize(cGreen, "x"); got != "x" {
		t.Errorf("color disabled: got %q, want plain %q", got, "x")
	}

	color.NoColor = false
	got := colorize(cGreen, "x")
	if !strings.Contains(got, "\x1b[32m") || !strings.Contains(got, "\x1b[0m") {
		t.Errorf("color enabled: %q not wrapped in green + reset", got)
	}

	// A nil colour is always a no-op, even with colour on.
	if got := colorize(nil, "x"); got != "x" {
		t.Errorf("nil colour: got %q, want %q", got, "x")
	}
}
