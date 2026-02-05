package errors

import (
	"errors"
	"fmt"
)

// Exit codes for forage-ctl
const (
	ExitSuccess           = 0
	ExitGeneralError      = 1
	ExitSandboxNotFound   = 2
	ExitTemplateNotFound  = 3
	ExitPortAllocation    = 4
	ExitContainerFailed   = 5
	ExitConfigError       = 6
	ExitJJError           = 7
	ExitSSHError          = 8
)

// ForageError is the base error type for forage-ctl
type ForageError struct {
	Code    int
	Message string
	Cause   error
}

func (e *ForageError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *ForageError) Unwrap() error {
	return e.Cause
}

// ExitCode returns the exit code for this error
func (e *ForageError) ExitCode() int {
	return e.Code
}

// New creates a new ForageError
func New(code int, message string) *ForageError {
	return &ForageError{
		Code:    code,
		Message: message,
	}
}

// Wrap wraps an existing error with a ForageError
func Wrap(code int, message string, cause error) *ForageError {
	return &ForageError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// Common error constructors

// SandboxNotFound returns an error for a missing sandbox
func SandboxNotFound(name string) *ForageError {
	return New(ExitSandboxNotFound, fmt.Sprintf("sandbox not found: %s", name))
}

// TemplateNotFound returns an error for a missing template
func TemplateNotFound(name string) *ForageError {
	return New(ExitTemplateNotFound, fmt.Sprintf("template not found: %s", name))
}

// PortAllocationFailed returns an error for port allocation failure
func PortAllocationFailed(cause error) *ForageError {
	return Wrap(ExitPortAllocation, "failed to allocate port", cause)
}

// ContainerFailed returns an error for container operations
func ContainerFailed(op string, cause error) *ForageError {
	return Wrap(ExitContainerFailed, fmt.Sprintf("container %s failed", op), cause)
}

// ConfigError returns an error for configuration issues
func ConfigError(message string, cause error) *ForageError {
	return Wrap(ExitConfigError, message, cause)
}

// JJError returns an error for jj operations
func JJError(message string, cause error) *ForageError {
	return Wrap(ExitJJError, message, cause)
}

// SSHError returns an error for SSH operations
func SSHError(message string, cause error) *ForageError {
	return Wrap(ExitSSHError, message, cause)
}

// SandboxNotRunning returns an error when a sandbox exists but is not running
func SandboxNotRunning(name string) *ForageError {
	return New(ExitGeneralError, fmt.Sprintf("sandbox %s is not running", name))
}

// WorkspaceError returns an error for workspace operations
func WorkspaceError(op string, cause error) *ForageError {
	return Wrap(ExitGeneralError, fmt.Sprintf("workspace %s failed", op), cause)
}

// ValidationError returns an error for input validation failures
func ValidationError(message string) *ForageError {
	return New(ExitGeneralError, message)
}

// GetExitCode extracts the exit code from an error
func GetExitCode(err error) int {
	var forageErr *ForageError
	if errors.As(err, &forageErr) {
		return forageErr.ExitCode()
	}
	return ExitGeneralError
}

// Is checks if an error is of a specific type
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As finds the first error in err's chain that matches target
func As(err error, target any) bool {
	return errors.As(err, target)
}
