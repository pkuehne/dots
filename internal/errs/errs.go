// Package errs defines user-facing error types for dots.
// Every error has a Msg and an optional Hint shown below the error line.
package errs

import (
	"fmt"
	"strings"
)

// DotsError is the base user-facing error type.
type DotsError struct {
	Msg  string
	Hint string
}

func (e *DotsError) Error() string { return e.Msg }

// Render returns the formatted multi-line string for stderr output.
func (e *DotsError) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n✗ %s", e.Msg)
	if e.Hint != "" {
		b.WriteString("\n")
		for _, line := range strings.Split(e.Hint, "\n") {
			fmt.Fprintf(&b, "\n  %s", line)
		}
	}
	return b.String()
}

// ConfigError is a configuration parse or validation error.
type ConfigError struct{ DotsError }

// ToolInstallError is returned when a tool installation fails.
type ToolInstallError struct{ DotsError }

func New(msg, hint string) *DotsError           { return &DotsError{Msg: msg, Hint: hint} }
func NewConfig(msg, hint string) *ConfigError    { return &ConfigError{DotsError{Msg: msg, Hint: hint}} }
func NewTool(msg, hint string) *ToolInstallError { return &ToolInstallError{DotsError{Msg: msg, Hint: hint}} }

// Unwrap returns the underlying *DotsError for any dots error type, so callers
// can call Render() without a type switch.
func Unwrap(err error) (*DotsError, bool) {
	switch e := err.(type) {
	case *DotsError:
		return e, true
	case *ConfigError:
		return &e.DotsError, true
	case *ToolInstallError:
		return &e.DotsError, true
	}
	return nil, false
}
