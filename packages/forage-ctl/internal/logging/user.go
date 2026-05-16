package logging

import (
	"fmt"
	"os"
)

// User-facing output functions with emoji prefixes.
// These write to stdout/stderr directly for CLI output,
// separate from the structured debug logging.

// UserInfo prints an info message to stdout.
func UserInfo(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "ℹ "+format+"\n", args...)
}

// UserSuccess prints a success message to stdout.
func UserSuccess(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "✓ "+format+"\n", args...)
}

// UserWarning prints a warning message to stderr.
func UserWarning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "⚠ "+format+"\n", args...)
}

// UserError prints an error message to stderr.
func UserError(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
}
