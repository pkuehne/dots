package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect_ReturnsKnownValue(t *testing.T) {
	p := Detect()
	known := map[string]bool{"linux": true, "darwin": true, "windows": true, "termux": true}
	if !known[p] {
		t.Errorf("Detect() = %q, want one of linux/darwin/windows/termux", p)
	}
}

func TestArch_ReturnsKnownValue(t *testing.T) {
	a := Arch()
	if a == "" {
		t.Error("Arch() returned empty string")
	}
}

func TestPlatforms_ContainsDetect(t *testing.T) {
	plats := Platforms()
	p := Detect()
	found := false
	for _, v := range plats {
		if v == p {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Platforms() = %v, does not contain Detect() = %q", plats, p)
	}
}

func TestIsWSL_FakeProcVersion(t *testing.T) {
	// Override /proc/version by writing a fake file and monkey-patching
	// procVersionPath (if exported), or test via the exported function directly
	// using a temp file approach — IsWSL reads /proc/version directly, so we
	// test the two code paths by writing fake content via a helper.
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"wsl2 kernel", "Linux version 5.15.90.1-microsoft-standard-WSL2", true},
		{"wsl1 marker", "Linux version 4.4.0-WSL", true},
		{"plain linux", "Linux version 5.15.0-76-generic (buildd@lcy02-amd64-059)", false},
		{"darwin", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Write fake /proc/version content to a temp file and point the
			// package at it via the exported procVersionFile variable.
			tmp := filepath.Join(t.TempDir(), "version")
			if tc.content != "" {
				if err := os.WriteFile(tmp, []byte(tc.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got := isWSLFile(tmp)
			if got != tc.want {
				t.Errorf("isWSLFile(%q content=%q) = %v, want %v", tmp, tc.content, got, tc.want)
			}
		})
	}
}
