// Package platform detects the current OS, architecture, and hostname.
package platform

import (
	"os"
	"runtime"
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
