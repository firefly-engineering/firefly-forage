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

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/system"
)

// Default SSH configuration values.
const (
	DefaultUser           = "agent"
	DefaultConnectTimeout = 2
)

// Options configures SSH connection parameters.
type Options struct {
	User               string
	Host               string
	StrictHostKeyCheck bool
	KnownHostsFile     string
	ConnectTimeout     int
	BatchMode          bool
	RequestTTY         bool
}

// DefaultOptions returns Options with sensible defaults for sandbox connections.
// The host parameter should be the container IP (e.g., "10.100.1.2").
func DefaultOptions(host string) Options {
	return Options{
		User:               DefaultUser,
		Host:               host,
		StrictHostKeyCheck: false,
		KnownHostsFile:     "/dev/null",
		ConnectTimeout:     DefaultConnectTimeout,
		BatchMode:          false,
		RequestTTY:         false,
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
	var args []string

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
func Exec(host string, args ...string) error {
	opts := DefaultOptions(host)
	sshArgs := opts.BuildArgs(args...)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// ExecWithOutput executes a command and returns output.
func ExecWithOutput(host string, args ...string) (string, error) {
	opts := DefaultOptions(host).WithBatchMode()
	sshArgs := opts.BuildArgs(args...)

	cmd := exec.Command("ssh", sshArgs...)
	output, err := cmd.Output()
	return string(output), err
}

// ExecWithStdin executes a command with stdin input.
func ExecWithStdin(host string, stdin string, args ...string) error {
	opts := DefaultOptions(host).WithBatchMode()
	sshArgs := opts.BuildArgs(args...)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = bytes.NewReader([]byte(stdin))
	return cmd.Run()
}

// Interactive starts an interactive SSH session.
func Interactive(host string, command string) error {
	opts := DefaultOptions(host).WithTTY()
	sshArgs := opts.BuildArgs(command)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ReplaceWithSession replaces the current process with an SSH session.
// This uses syscall.Exec and does not return on success.
func ReplaceWithSession(host string, command string) error {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found: %w", err)
	}

	opts := DefaultOptions(host).WithTTY()
	sshArgs := opts.BuildArgsWithArgv(command)

	return syscall.Exec(sshPath, sshArgs, system.SafeEnviron())
}

// CheckConnection checks if SSH is reachable.
func CheckConnection(host string) bool {
	opts := DefaultOptions(host).WithBatchMode()
	sshArgs := opts.BuildArgs("true")

	cmd := exec.Command("ssh", sshArgs...)
	return cmd.Run() == nil
}
