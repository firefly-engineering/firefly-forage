package ssh

import (
	"strings"
	"testing"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions("10.100.1.2")

	if opts.Host != "10.100.1.2" {
		t.Errorf("Host = %q, want %q", opts.Host, "10.100.1.2")
	}
	if opts.User != DefaultUser {
		t.Errorf("User = %q, want %q", opts.User, DefaultUser)
	}
	if opts.StrictHostKeyCheck {
		t.Error("StrictHostKeyCheck should be false by default")
	}
	if opts.ConnectTimeout != DefaultConnectTimeout {
		t.Errorf("ConnectTimeout = %d, want %d", opts.ConnectTimeout, DefaultConnectTimeout)
	}
	if opts.BatchMode {
		t.Error("BatchMode should be false by default")
	}
	if opts.RequestTTY {
		t.Error("RequestTTY should be false by default")
	}
}

func TestOptionsWithBatchMode(t *testing.T) {
	opts := DefaultOptions("10.100.1.2").WithBatchMode()

	if !opts.BatchMode {
		t.Error("WithBatchMode should enable batch mode")
	}
	// Ensure original host is preserved
	if opts.Host != "10.100.1.2" {
		t.Errorf("Host = %q, want %q", opts.Host, "10.100.1.2")
	}
}

func TestOptionsWithTTY(t *testing.T) {
	opts := DefaultOptions("10.100.1.2").WithTTY()

	if !opts.RequestTTY {
		t.Error("WithTTY should enable TTY")
	}
}

func TestOptionsWithTimeout(t *testing.T) {
	opts := DefaultOptions("10.100.1.2").WithTimeout(10)

	if opts.ConnectTimeout != 10 {
		t.Errorf("ConnectTimeout = %d, want 10", opts.ConnectTimeout)
	}
}

func TestOptionsChaining(t *testing.T) {
	opts := DefaultOptions("10.100.1.2").
		WithBatchMode().
		WithTTY().
		WithTimeout(5)

	if !opts.BatchMode {
		t.Error("BatchMode should be true")
	}
	if !opts.RequestTTY {
		t.Error("RequestTTY should be true")
	}
	if opts.ConnectTimeout != 5 {
		t.Errorf("ConnectTimeout = %d, want 5", opts.ConnectTimeout)
	}
}

func TestDestination(t *testing.T) {
	opts := DefaultOptions("10.100.1.2")

	dest := opts.Destination()
	expected := "agent@10.100.1.2"

	if dest != expected {
		t.Errorf("Destination() = %q, want %q", dest, expected)
	}
}

func TestBaseArgs(t *testing.T) {
	tests := []struct {
		name     string
		opts     Options
		contains []string
		excludes []string
	}{
		{
			name: "default options",
			opts: DefaultOptions("10.100.1.2"),
			contains: []string{
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "ConnectTimeout=2",
			},
			excludes: []string{
				"BatchMode",
				"-t",
				"-p", // No port flag - using standard port 22
			},
		},
		{
			name: "with batch mode",
			opts: DefaultOptions("10.100.1.2").WithBatchMode(),
			contains: []string{
				"-o", "BatchMode=yes",
			},
		},
		{
			name: "with TTY",
			opts: DefaultOptions("10.100.1.2").WithTTY(),
			contains: []string{
				"-t",
			},
		},
		{
			name: "custom timeout",
			opts: DefaultOptions("10.100.1.2").WithTimeout(30),
			contains: []string{
				"-o", "ConnectTimeout=30",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.opts.BaseArgs()
			argsStr := strings.Join(args, " ")

			for _, want := range tt.contains {
				if !strings.Contains(argsStr, want) {
					t.Errorf("BaseArgs() missing %q, got: %v", want, args)
				}
			}

			for _, exclude := range tt.excludes {
				if strings.Contains(argsStr, exclude) {
					t.Errorf("BaseArgs() should not contain %q, got: %v", exclude, args)
				}
			}
		})
	}
}

func TestBuildArgs(t *testing.T) {
	opts := DefaultOptions("10.100.1.2")
	args := opts.BuildArgs("ls", "-la")

	// Should end with destination and command
	if len(args) < 3 {
		t.Fatalf("BuildArgs() returned too few args: %v", args)
	}

	// Check destination is present
	argsStr := strings.Join(args, " ")
	if !strings.Contains(argsStr, "agent@10.100.1.2") {
		t.Errorf("BuildArgs() should contain destination, got: %v", args)
	}

	// Check command is at the end
	if args[len(args)-2] != "ls" || args[len(args)-1] != "-la" {
		t.Errorf("BuildArgs() command not at end, got: %v", args)
	}
}

func TestBuildArgsNoCommand(t *testing.T) {
	opts := DefaultOptions("10.100.1.2")
	args := opts.BuildArgs()

	// Should end with destination
	if len(args) == 0 {
		t.Fatal("BuildArgs() returned empty args")
	}

	lastArg := args[len(args)-1]
	if lastArg != "agent@10.100.1.2" {
		t.Errorf("BuildArgs() should end with destination, got: %q", lastArg)
	}
}

func TestBuildArgsWithArgv(t *testing.T) {
	opts := DefaultOptions("10.100.1.2")
	args := opts.BuildArgsWithArgv("echo", "hello")

	// First arg should be "ssh"
	if len(args) == 0 || args[0] != "ssh" {
		t.Errorf("BuildArgsWithArgv() should start with 'ssh', got: %v", args)
	}

	// Check command is present
	argsStr := strings.Join(args, " ")
	if !strings.Contains(argsStr, "echo") || !strings.Contains(argsStr, "hello") {
		t.Errorf("BuildArgsWithArgv() should contain command, got: %v", args)
	}
}

func TestCheckConnection(t *testing.T) {
	// This test verifies the function exists and handles errors gracefully
	// Actual connection testing would require a running SSH server
	result := CheckConnection("192.0.2.1") // TEST-NET-1 address, should fail
	if result {
		t.Error("CheckConnection should return false for unreachable host")
	}
}
