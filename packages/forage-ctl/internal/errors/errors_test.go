package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestForageError_Error(t *testing.T) {
	tests := []struct {
		name    string
		err     *ForageError
		wantMsg string
	}{
		{
			name:    "without cause",
			err:     New(ExitGeneralError, "something went wrong"),
			wantMsg: "something went wrong",
		},
		{
			name:    "with cause",
			err:     Wrap(ExitGeneralError, "operation failed", fmt.Errorf("underlying error")),
			wantMsg: "operation failed: underlying error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

func TestForageError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("root cause")
	err := Wrap(ExitGeneralError, "wrapped", cause)

	if unwrapped := err.Unwrap(); unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}

	// Without cause
	errNoCause := New(ExitGeneralError, "no cause")
	if unwrapped := errNoCause.Unwrap(); unwrapped != nil {
		t.Errorf("Unwrap() = %v, want nil", unwrapped)
	}
}

func TestForageError_ExitCode(t *testing.T) {
	tests := []struct {
		code int
		name string
	}{
		{ExitSuccess, "success"},
		{ExitGeneralError, "general"},
		{ExitSandboxNotFound, "sandbox not found"},
		{ExitTemplateNotFound, "template not found"},
		{ExitPortAllocation, "port allocation"},
		{ExitContainerFailed, "container failed"},
		{ExitConfigError, "config error"},
		{ExitJJError, "jj error"},
		{ExitSSHError, "ssh error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.code, "test")
			if got := err.ExitCode(); got != tt.code {
				t.Errorf("ExitCode() = %d, want %d", got, tt.code)
			}
		})
	}
}

func TestSandboxNotFound(t *testing.T) {
	err := SandboxNotFound("my-sandbox")

	if err.Code != ExitSandboxNotFound {
		t.Errorf("Code = %d, want %d", err.Code, ExitSandboxNotFound)
	}

	if err.Message != "sandbox not found: my-sandbox" {
		t.Errorf("Message = %q, want %q", err.Message, "sandbox not found: my-sandbox")
	}
}

func TestTemplateNotFound(t *testing.T) {
	err := TemplateNotFound("claude")

	if err.Code != ExitTemplateNotFound {
		t.Errorf("Code = %d, want %d", err.Code, ExitTemplateNotFound)
	}

	if err.Message != "template not found: claude" {
		t.Errorf("Message = %q, want %q", err.Message, "template not found: claude")
	}
}

func TestPortAllocationFailed(t *testing.T) {
	cause := fmt.Errorf("no ports available")
	err := PortAllocationFailed(cause)

	if err.Code != ExitPortAllocation {
		t.Errorf("Code = %d, want %d", err.Code, ExitPortAllocation)
	}

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestContainerFailed(t *testing.T) {
	cause := fmt.Errorf("nspawn error")
	err := ContainerFailed("create", cause)

	if err.Code != ExitContainerFailed {
		t.Errorf("Code = %d, want %d", err.Code, ExitContainerFailed)
	}

	if err.Message != "container create failed" {
		t.Errorf("Message = %q, want %q", err.Message, "container create failed")
	}

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestConfigError(t *testing.T) {
	cause := fmt.Errorf("invalid json")
	err := ConfigError("failed to parse config", cause)

	if err.Code != ExitConfigError {
		t.Errorf("Code = %d, want %d", err.Code, ExitConfigError)
	}

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestJJError(t *testing.T) {
	cause := fmt.Errorf("workspace conflict")
	err := JJError("workspace creation failed", cause)

	if err.Code != ExitJJError {
		t.Errorf("Code = %d, want %d", err.Code, ExitJJError)
	}

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestSSHError(t *testing.T) {
	cause := fmt.Errorf("connection refused")
	err := SSHError("failed to connect", cause)

	if err.Code != ExitSSHError {
		t.Errorf("Code = %d, want %d", err.Code, ExitSSHError)
	}

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestGetExitCode(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{
			name:     "ForageError",
			err:      SandboxNotFound("test"),
			wantCode: ExitSandboxNotFound,
		},
		{
			name:     "wrapped ForageError",
			err:      fmt.Errorf("outer: %w", TemplateNotFound("test")),
			wantCode: ExitTemplateNotFound,
		},
		{
			name:     "regular error",
			err:      fmt.Errorf("some error"),
			wantCode: ExitGeneralError,
		},
		{
			name:     "nil error",
			err:      nil,
			wantCode: ExitGeneralError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetExitCode(tt.err); got != tt.wantCode {
				t.Errorf("GetExitCode() = %d, want %d", got, tt.wantCode)
			}
		})
	}
}

func TestIs(t *testing.T) {
	target := fmt.Errorf("target error")
	wrapped := fmt.Errorf("wrapped: %w", target)

	if !Is(wrapped, target) {
		t.Error("Is() should return true for wrapped error")
	}

	other := fmt.Errorf("other error")
	if Is(wrapped, other) {
		t.Error("Is() should return false for different error")
	}
}

func TestAs(t *testing.T) {
	forageErr := SandboxNotFound("test")
	wrapped := fmt.Errorf("wrapped: %w", forageErr)

	var target *ForageError
	if !As(wrapped, &target) {
		t.Error("As() should return true for wrapped ForageError")
	}

	if target.Code != ExitSandboxNotFound {
		t.Errorf("target.Code = %d, want %d", target.Code, ExitSandboxNotFound)
	}

	// Test with non-ForageError
	regularErr := fmt.Errorf("regular error")
	if As(regularErr, &target) {
		t.Error("As() should return false for non-ForageError")
	}
}

func TestErrorChaining(t *testing.T) {
	// Test that our errors work with standard error unwrapping
	root := fmt.Errorf("root cause")
	middle := Wrap(ExitConfigError, "config error", root)
	outer := fmt.Errorf("operation failed: %w", middle)

	// Should be able to find root cause
	if !errors.Is(outer, root) {
		t.Error("errors.Is should find root cause")
	}

	// Should be able to extract ForageError
	var forageErr *ForageError
	if !errors.As(outer, &forageErr) {
		t.Error("errors.As should find ForageError")
	}

	if forageErr.Code != ExitConfigError {
		t.Errorf("Code = %d, want %d", forageErr.Code, ExitConfigError)
	}
}
