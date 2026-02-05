package container

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/config"
)

// Status represents the container status
type Status string

const (
	StatusRunning  Status = "running"
	StatusStopped  Status = "stopped"
	StatusNotFound Status = "not-found"
)

// IsRunning checks if a container is running using machinectl
func IsRunning(sandboxName string) bool {
	containerName := config.ContainerName(sandboxName)
	cmd := exec.Command("machinectl", "show", containerName)
	return cmd.Run() == nil
}

// GetStatus returns the status of a sandbox
func GetStatus(sandboxName string, sandboxesDir string) Status {
	if !config.SandboxExists(sandboxesDir, sandboxName) {
		return StatusNotFound
	}
	if IsRunning(sandboxName) {
		return StatusRunning
	}
	return StatusStopped
}

// GetUptime returns the uptime of a running container
func GetUptime(sandboxName string) (string, error) {
	containerName := config.ContainerName(sandboxName)
	cmd := exec.Command("machinectl", "show", containerName, "-p", "Since", "--value")
	output, err := cmd.Output()
	if err != nil {
		return "unknown", err
	}

	since := strings.TrimSpace(string(output))
	if since == "" || since == "n/a" {
		return "unknown", nil
	}

	// Parse and calculate uptime
	// For now, return the raw value - we can improve this later
	return since, nil
}

// Stop stops a running container using machinectl
func Stop(extraContainerPath, sandboxName string) error {
	containerName := config.ContainerName(sandboxName)
	cmd := exec.Command("sudo", "machinectl", "stop", containerName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	return nil
}

// Start starts a stopped container using extra-container
func Start(extraContainerPath, sandboxName string, configPath string) error {
	cmd := exec.Command("sudo", extraContainerPath, "create", "--start", configPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	return nil
}

// Destroy destroys a container using extra-container
func Destroy(extraContainerPath, sandboxName string) error {
	containerName := config.ContainerName(sandboxName)
	cmd := exec.Command("sudo", extraContainerPath, "destroy", containerName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Ignore errors if container doesn't exist
		return nil
	}
	return nil
}

// ExecSSH executes a command in a sandbox via SSH
func ExecSSH(port int, args ...string) error {
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

// ExecSSHWithOutput executes a command and returns output
func ExecSSHWithOutput(port int, args ...string) (string, error) {
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

// SSHInteractive starts an interactive SSH session
func SSHInteractive(port int, command string) error {
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
