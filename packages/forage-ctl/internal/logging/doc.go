// Package logging provides logging utilities for forage-ctl.
//
// This package provides two categories of output:
//   - Debug logging: Structured logs for debugging (via slog)
//   - User output: Formatted messages for end users
//
// # Debug Logging
//
// Debug logs are written using slog and controlled by verbosity settings:
//
//	logging.Debug("creating sandbox", "name", name, "template", template)
//	logging.Warn("SSH timeout", "port", port, "timeout", timeout)
//
// # User Output
//
// User-facing messages are formatted with status indicators:
//
//	logging.UserInfo("Loading template %s...", templateName)
//	logging.UserSuccess("Sandbox %s created successfully", name)
//	logging.UserWarning("Port %d is already in use", port)
//	logging.UserError("Failed to create sandbox: %v", err)
//
// Output destinations:
//   - UserInfo, UserSuccess: stdout
//   - UserWarning, UserError: stderr
//
// # Status Indicators
//
// User functions prepend status indicators:
//   - ℹ (info)
//   - ✓ (success)
//   - ⚠ (warning)
//   - ✗ (error)
package logging
