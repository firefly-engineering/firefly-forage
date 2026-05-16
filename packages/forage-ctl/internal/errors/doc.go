// Package errors provides typed errors with exit codes for forage-ctl.
//
// # Error Types
//
// ForageError is the base error type that wraps an error with an exit code:
//
//	type ForageError struct {
//	    Code    int    // Exit code
//	    Message string // User-facing message
//	    Cause   error  // Wrapped error
//	}
//
// # Exit Codes
//
// Defined exit codes for different error categories:
//
//	ExitSuccess           = 0  // Success
//	ExitGeneralError      = 1  // General/unknown errors
//	ExitSandboxNotFound   = 2  // Sandbox does not exist
//	ExitTemplateNotFound  = 3  // Template does not exist
//	ExitPortAllocation    = 4  // Port allocation failure
//	ExitContainerFailed   = 5  // Container operation failed
//	ExitConfigError       = 6  // Configuration error
//	ExitJJError           = 7  // JJ operation failed
//	ExitSSHError          = 8  // SSH operation failed
//
// # Error Constructors
//
// Use the provided constructors for consistent error creation:
//
//	errors.SandboxNotFound("mybox")
//	errors.TemplateNotFound("claude")
//	errors.ContainerFailed("create", err)
//	errors.SSHError("connection failed", err)
//
// # Extracting Exit Codes
//
// Use GetExitCode to extract the exit code from an error chain:
//
//	if err != nil {
//	    os.Exit(errors.GetExitCode(err))
//	}
package errors
