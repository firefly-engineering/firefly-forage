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

// Exec executes a command in a sandbox via SSH
func Exec(port int, args ...string) error {
	sshArgs := []string{
		"-p", fmt.Sprintf("%d", port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"agent@localhost",
	}
	sshArgs = append(sshArgs, args...)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// ExecWithOutput executes a command and returns output
func ExecWithOutput(port int, args ...string) (string, error) {
	sshArgs := []string{
		"-p", fmt.Sprintf("%d", port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=2",
		"agent@localhost",
	}
	sshArgs = append(sshArgs, args...)

	cmd := exec.Command("ssh", sshArgs...)
	output, err := cmd.Output()
	return string(output), err
}

// ExecWithStdin executes a command with stdin input
func ExecWithStdin(port int, stdin string, args ...string) error {
	sshArgs := []string{
		"-p", fmt.Sprintf("%d", port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"agent@localhost",
	}
	sshArgs = append(sshArgs, args...)

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = bytes.NewReader([]byte(stdin))
	return cmd.Run()
}

// Interactive starts an interactive SSH session
func Interactive(port int, command string) error {
	sshArgs := []string{
		"-p", fmt.Sprintf("%d", port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-t", "agent@localhost",
		command,
	}

	cmd := exec.Command("ssh", sshArgs...)
	cmd.Stdin = nil // Will inherit from parent
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// ReplaceWithSession replaces the current process with an SSH session.
// This uses syscall.Exec and does not return on success.
func ReplaceWithSession(port int, command string) error {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found: %w", err)
	}

	sshArgs := []string{
		"ssh",
		"-p", fmt.Sprintf("%d", port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-t", "agent@localhost",
		command,
	}

	return syscall.Exec(sshPath, sshArgs, os.Environ())
}

// CheckConnection checks if SSH is reachable
func CheckConnection(port int) bool {
	args := []string{
		"-p", fmt.Sprintf("%d", port),
		"-o", "ConnectTimeout=2",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "BatchMode=yes",
		"agent@localhost", "true",
	}
	cmd := exec.Command("ssh", args...)
	return cmd.Run() == nil
}
