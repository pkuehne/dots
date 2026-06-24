package ui

import (
	"strings"
	"testing"

	"github.com/fatih/color"
)

func TestColorize_RespectsColorEnabled(t *testing.T) {
	orig := color.NoColor
	defer func() { color.NoColor = orig }()

	color.NoColor = true
	if got := Colorize(Green, "x"); got != "x" {
		t.Errorf("color disabled: got %q, want plain %q", got, "x")
	}

	color.NoColor = false
	got := Colorize(Green, "x")
	if !strings.Contains(got, "\x1b[32m") || !strings.Contains(got, "\x1b[0m") {
		t.Errorf("color enabled: %q not wrapped in green + reset", got)
	}

	// A nil colour is always a no-op, even with colour on.
	if got := Colorize(nil, "x"); got != "x" {
		t.Errorf("nil colour: got %q, want %q", got, "x")
	}
}

func TestStyleFor(t *testing.T) {
	tests := []struct {
		action    string
		wantLabel string
		wantVerb  string // present-tense form for dry-run "would <verb>"
	}{
		{"linked", "linked", "link"},
		{"link", "linked", "link"},
		{"installed", "installed", "install"},
		{"install", "installed", "install"},
		{"cloned", "cloned", "clone"},
		{"clone", "cloned", "clone"},
		{"wrote", "wrote", "write"},
		{"write", "wrote", "write"},
		{"removed", "removed", "remove"},
		{"remove", "removed", "remove"},
		{"backed up & wrote", "backed up & wrote", ""},
		{"diff", "drifted", "update"},
		{"present", "present", ""},
		{"copied", "copied", "copy"},
		{"copy", "copied", "copy"},
		{"decrypted", "decrypted", "decrypt"},
		{"decrypt", "decrypted", "decrypt"},
		{"unchanged", "unchanged", ""},
		{"missing", "missing", ""},
		{"skipped", "skipped", ""},
		{"skipped (profile)", "skipped (profile)", ""},
	}
	for _, tc := range tests {
		st := styleFor(tc.action)
		if st.label != tc.wantLabel {
			t.Errorf("styleFor(%q).label = %q, want %q", tc.action, st.label, tc.wantLabel)
		}
		if st.verb != tc.wantVerb {
			t.Errorf("styleFor(%q).verb = %q, want %q", tc.action, st.verb, tc.wantVerb)
		}
		if st.icon == "" {
			t.Errorf("styleFor(%q) has no icon", tc.action)
		}
	}
}
