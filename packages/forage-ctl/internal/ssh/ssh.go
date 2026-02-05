// Package ssh provides SSH connection utilities for sandbox access.
// These functions work regardless of the container runtime since
// all sandboxes have SSH enabled.
package ssh

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// Default SSH configuration values.
const (
	DefaultUser           = "agent"
	DefaultHost           = "localhost"
	DefaultConnectTimeout = 2
)

// Options configures SSH connection parameters.
type Options struct {
	Port              int
	User              string
	Host              string
	StrictHostKeyCheck bool
	KnownHostsFile    string
	ConnectTimeout    int
	BatchMode         bool
	RequestTTY        bool
}

// DefaultOptions returns Options with sensible defaults for sandbox connections.
func DefaultOptions(port int) Options {
	return Options{
		Port:              port,
		User:              DefaultUser,
		Host:              DefaultHost,
		StrictHostKeyCheck: false,
		KnownHostsFile:    "/dev/null",
		ConnectTimeout:    DefaultConnectTimeout,
		BatchMode:         false,
		RequestTTY:        false,
	}
}

// WithBatchMode returns a copy with batch mode enabled.
func (o Options) WithBatchMode() Options {
	o.BatchMode = true
	return o
}

// WithTTY returns a copy with TTY requested.
func (o Options) WithTTY() Options {
	o.RequestTTY = true
	return o
}

// WithTimeout returns a copy with the specified connect timeout.
func (o Options) WithTimeout(seconds int) Options {
	o.ConnectTimeout = seconds
	return o
}

// BaseArgs returns the common SSH arguments (options only, no user@host).
func (o Options) BaseArgs() []string {
	args := []string{
		"-p", fmt.Sprintf("%d", o.Port),
	}

	if !o.StrictHostKeyCheck {
		args = append(args, "-o", "StrictHostKeyChecking=no")
	}

	if o.KnownHostsFile != "" {
		args = append(args, "-o", fmt.Sprintf("UserKnownHostsFile=%s", o.KnownHostsFile))
	}

	if o.BatchMode {
		args = append(args, "-o", "BatchMode=yes")
	}

	if o.ConnectTimeout > 0 {
		args = append(args, "-o", fmt.Sprintf("ConnectTimeout=%d", o.ConnectTimeout))
	}

	if o.RequestTTY {
		args = append(args, "-t")
	}

	return args
}

// Destination returns the user@host string.
func (o Options) Destination() string {
	return fmt.Sprintf("%s@%s", o.User, o.Host)
}

// BuildArgs returns complete SSH arguments for executing a command.
func (o Options) BuildArgs(command ...string) []string {
	args := o.BaseArgs()
	args = append(args, o.Destination())
	args = append(args, command...)
	return args
}

// BuildArgsWithArgv returns complete SSH arguments including "ssh" as argv[0].
// Used for syscall.Exec which requires the program name in argv.
func (o Options) BuildArgsWithArgv(command ...string) []string {
	args := []string{"ssh"}
	args = append(args, o.BuildArgs(command...)...)
	return args
}

// --- Convenience functions using the builder ---

// Exec executes a command in a sandbox via SSH.
func Exec(port int, args ...string) error {
	opts := DefaultOptions(port)
	sshArgs := opts.BuildArgs(args...)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// ExecWithOutput executes a command and returns output.
func ExecWithOutput(port int, args ...string) (string, error) {
	opts := DefaultOptions(port).WithBatchMode()
	sshArgs := opts.BuildArgs(args...)

	cmd := exec.Command("ssh", sshArgs...)
	output, err := cmd.Output()
	return string(output), err
}

// ExecWithStdin executes a command with stdin input.
func ExecWithStdin(port int, stdin string, args ...string) error {
	opts := DefaultOptions(port).WithBatchMode()
	sshArgs := opts.BuildArgs(args...)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = bytes.NewReader([]byte(stdin))
	return cmd.Run()
}

// Interactive starts an interactive SSH session.
func Interactive(port int, command string) error {
	opts := DefaultOptions(port).WithTTY()
	sshArgs := opts.BuildArgs(command)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ReplaceWithSession replaces the current process with an SSH session.
// This uses syscall.Exec and does not return on success.
func ReplaceWithSession(port int, command string) error {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found: %w", err)
	}

	opts := DefaultOptions(port).WithTTY()
	sshArgs := opts.BuildArgsWithArgv(command)

	return syscall.Exec(sshPath, sshArgs, os.Environ())
}

// CheckConnection checks if SSH is reachable.
func CheckConnection(port int) bool {
	opts := DefaultOptions(port).WithBatchMode()
	sshArgs := opts.BuildArgs("true")

	cmd := exec.Command("ssh", sshArgs...)
	return cmd.Run() == nil
}
