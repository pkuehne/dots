// Package platform detects the current OS, architecture, and hostname.
package platform

import (
	"os"
	"runtime"
	"strings"
)

// Detect returns one of: "linux", "darwin", "windows", "termux".
func Detect() string {
	if _, err := os.Stat("/data/data/com.termux"); err == nil {
		return "termux"
	}
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

// OSName returns the OS name used in asset patterns ("linux" for both linux and termux).
func OSName() string {
	if Detect() == "termux" {
		return "linux"
	}
	return Detect()
}

// Arch returns the native machine architecture string (e.g. "x86_64", "aarch64").
func Arch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armv7"
	default:
		return runtime.GOARCH
	}
}

// GoArch returns the Go-style architecture name (e.g. "amd64", "arm64").
// This is just runtime.GOARCH, exposed for symmetry with Arch().
func GoArch() string { return runtime.GOARCH }

// Hostname returns the machine hostname, or empty string on error.
func Hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}

// IsWSL reports whether the process is running inside Windows Subsystem for
// Linux. It checks /proc/version for the "microsoft" or "WSL" strings, which
// are present in both WSL1 and WSL2 kernels.
func IsWSL() bool {
	return isWSLFile("/proc/version")
}

func isWSLFile(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "microsoft") || strings.Contains(lower, "wsl")
}

// Platforms returns all platform tags that apply to the current environment.
// On WSL this is ["linux", "wsl"]; on other systems it is [Detect()].
// Use this for multi-tag matching (e.g. env.when.only, files.d/ scoping).
func Platforms() []string {
	p := Detect()
	if p == "linux" && IsWSL() {
		return []string{"linux", "wsl"}
	}
	return []string{p}
}
