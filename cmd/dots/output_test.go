package main

import (
	"strings"
	"testing"
)

func TestColorize_RespectsUseColor(t *testing.T) {
	orig := useColor
	defer func() { useColor = orig }()

	useColor = false
	if got := colorize(cGreen, "x"); got != "x" {
		t.Errorf("color disabled: got %q, want plain %q", got, "x")
	}

	useColor = true
	got := colorize(cGreen, "x")
	if !strings.HasPrefix(got, cGreen) || !strings.HasSuffix(got, cReset) {
		t.Errorf("color enabled: %q not wrapped in green + reset", got)
	}

	// An empty code is always a no-op, even with colour on.
	if got := colorize("", "x"); got != "x" {
		t.Errorf("empty code: got %q, want %q", got, "x")
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
