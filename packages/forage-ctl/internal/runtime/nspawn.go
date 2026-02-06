package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
)

// NspawnRuntime implements the Runtime interface using systemd-nspawn
// via extra-container for NixOS systems.
type NspawnRuntime struct {
	// ExtraContainerPath is the path to the extra-container binary
	ExtraContainerPath string

	// ContainerPrefix is prepended to sandbox names to form container names
	ContainerPrefix string

	// SandboxesDir is the directory containing sandbox metadata files
	// Used for looking up SSH ports from persisted metadata
	SandboxesDir string
}

// NewNspawnRuntime creates a new nspawn runtime with the given configuration
func NewNspawnRuntime(extraContainerPath, containerPrefix, sandboxesDir string) *NspawnRuntime {
	return &NspawnRuntime{
		ExtraContainerPath: extraContainerPath,
		ContainerPrefix:    containerPrefix,
		SandboxesDir:       sandboxesDir,
	}
}

// containerName returns the full container name for a sandbox
func (r *NspawnRuntime) containerName(sandboxName string) string {
	return r.ContainerPrefix + sandboxName
}

// Name returns the runtime identifier
func (r *NspawnRuntime) Name() string {
	return "nspawn"
}

// Create creates a new container using extra-container
func (r *NspawnRuntime) Create(ctx context.Context, opts CreateOptions) error {
	logging.Debug("creating container", "name", opts.Name, "config", opts.ConfigPath)

	args := []string{r.ExtraContainerPath, "create"}
	if opts.Start {
		args = append(args, "--start")
	}
	args = append(args, opts.ConfigPath)

	cmd := exec.CommandContext(ctx, "sudo", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("extra-container create failed: %w", err)
	}

	// SSH port is persisted in sandbox metadata by the caller
	return nil
}

// Start starts an existing container
func (r *NspawnRuntime) Start(ctx context.Context, name string) error {
	containerName := r.containerName(name)
	logging.Debug("starting container", "container", containerName)

	cmd := exec.CommandContext(ctx, "sudo", "machinectl", "start", containerName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("machinectl start failed: %w", err)
	}

	return nil
}

// Stop stops a running container
func (r *NspawnRuntime) Stop(ctx context.Context, name string) error {
	containerName := r.containerName(name)
	logging.Debug("stopping container", "container", containerName)

	cmd := exec.CommandContext(ctx, "sudo", "machinectl", "stop", containerName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("machinectl stop failed: %w", err)
	}

	return nil
}

// Destroy stops and removes a container
func (r *NspawnRuntime) Destroy(ctx context.Context, name string) error {
	containerName := r.containerName(name)
	logging.Debug("destroying container", "container", containerName)

	cmd := exec.CommandContext(ctx, "sudo", r.ExtraContainerPath, "destroy", containerName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Ignore errors if container doesn't exist
		logging.Debug("destroy returned error (may be expected)", "error", err, "stderr", stderr.String())
		return nil
	}

	return nil
}

// IsRunning checks if a container is currently running
func (r *NspawnRuntime) IsRunning(ctx context.Context, name string) (bool, error) {
	containerName := r.containerName(name)

	cmd := exec.CommandContext(ctx, "machinectl", "show", containerName)
	err := cmd.Run()

	return err == nil, nil
}

// Status returns detailed status of a container
func (r *NspawnRuntime) Status(ctx context.Context, name string) (*ContainerInfo, error) {
	containerName := r.containerName(name)

	info := &ContainerInfo{
		Name:   name,
		Status: StatusNotFound,
	}

	// Check if container exists
	cmd := exec.CommandContext(ctx, "machinectl", "show", containerName, "-p", "State", "--value")
	output, err := cmd.Output()
	if err != nil {
		return info, nil
	}

	state := strings.TrimSpace(string(output))
	switch state {
	case "running":
		info.Status = StatusRunning
	case "stopped", "":
		info.Status = StatusStopped
	default:
		info.Status = StatusUnknown
	}

	// Get start time if running
	if info.Status == StatusRunning {
		cmd = exec.CommandContext(ctx, "machinectl", "show", containerName, "-p", "Since", "--value")
		output, err = cmd.Output()
		if err == nil {
			info.StartedAt = strings.TrimSpace(string(output))
		}

		// Get IP address
		cmd = exec.CommandContext(ctx, "machinectl", "show", containerName, "-p", "IPAddress", "--value")
		output, err = cmd.Output()
		if err == nil {
			info.IPAddress = strings.TrimSpace(string(output))
		}
	}

	return info, nil
}

// Exec executes a command inside a container
func (r *NspawnRuntime) Exec(ctx context.Context, name string, command []string, opts ExecOptions) (*ExecResult, error) {
	containerName := r.containerName(name)

	args := []string{"machinectl", "shell"}
	if opts.User != "" {
		args = append(args, fmt.Sprintf("%s@%s", opts.User, containerName))
	} else {
		args = append(args, containerName)
	}
	args = append(args, "--")
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, "sudo", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}

	err := cmd.Run()

	result := &ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return result, fmt.Errorf("exec failed: %w", err)
		}
	}

	return result, nil
}

// ExecInteractive executes a command with an interactive TTY
func (r *NspawnRuntime) ExecInteractive(ctx context.Context, name string, command []string, opts ExecOptions) error {
	containerName := r.containerName(name)

	machinectlPath, err := exec.LookPath("machinectl")
	if err != nil {
		return fmt.Errorf("machinectl not found: %w", err)
	}

	args := []string{"machinectl", "shell"}
	if opts.User != "" {
		args = append(args, fmt.Sprintf("%s@%s", opts.User, containerName))
	} else {
		args = append(args, containerName)
	}

	if len(command) > 0 {
		args = append(args, "--")
		args = append(args, command...)
	}

	return syscall.Exec(machinectlPath, args, os.Environ())
}

// List returns all containers managed by this runtime
func (r *NspawnRuntime) List(ctx context.Context) ([]*ContainerInfo, error) {
	cmd := exec.CommandContext(ctx, "machinectl", "list", "--no-legend", "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("machinectl list failed: %w", err)
	}

	var containers []*ContainerInfo
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}

		name := fields[0]
		// Only include containers with our prefix
		if !strings.HasPrefix(name, r.ContainerPrefix) {
			continue
		}

		// Strip prefix to get sandbox name
		sandboxName := strings.TrimPrefix(name, r.ContainerPrefix)

		info, _ := r.Status(ctx, sandboxName)
		if info != nil {
			containers = append(containers, info)
		}
	}

	return containers, nil
}

// SSHHost returns the container IP address for SSH connections.
// The container IP is derived from the network slot in the metadata.
func (r *NspawnRuntime) SSHHost(ctx context.Context, name string) (string, error) {
	if r.SandboxesDir == "" {
		return "", fmt.Errorf("sandboxes directory not configured")
	}

	metadata, err := config.LoadSandboxMetadata(r.SandboxesDir, name)
	if err != nil {
		return "", fmt.Errorf("failed to load sandbox metadata: %w", err)
	}

	if metadata.NetworkSlot == 0 {
		return "", fmt.Errorf("no network slot configured for sandbox %s", name)
	}

	return metadata.ContainerIP(), nil
}

// SSHExec executes a command via SSH
func (r *NspawnRuntime) SSHExec(ctx context.Context, name string, command []string, opts ExecOptions) (*ExecResult, error) {
	host, err := r.SSHHost(ctx, name)
	if err != nil {
		return nil, err
	}
	return r.SSHExecWithHost(ctx, host, command, opts)
}

// SSHExecWithHost executes a command via SSH with a specific host
func (r *NspawnRuntime) SSHExecWithHost(ctx context.Context, host string, command []string, opts ExecOptions) (*ExecResult, error) {
	// Build SSH options using the builder
	sshOpts := ssh.DefaultOptions(host).WithBatchMode()

	// Override user if specified
	if opts.User != "" {
		sshOpts.User = opts.User
	}

	sshArgs := sshOpts.BuildArgs(command...)
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if opts.Stdin != nil {
		cmd.Stdin = opts.Stdin
	}

	err := cmd.Run()

	result := &ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return result, err
		}
	}

	return result, nil
}

// SSHInteractive starts an interactive SSH session
func (r *NspawnRuntime) SSHInteractive(ctx context.Context, name string, command string) error {
	host, err := r.SSHHost(ctx, name)
	if err != nil {
		return err
	}
	return r.SSHInteractiveWithHost(host, command)
}

// SSHInteractiveWithHost starts an interactive SSH session with a specific host
func (r *NspawnRuntime) SSHInteractiveWithHost(host string, command string) error {
	return ssh.ReplaceWithSession(host, command)
}

// Ensure NspawnRuntime implements Runtime
var _ Runtime = (*NspawnRuntime)(nil)
