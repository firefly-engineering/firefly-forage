package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
)

// DockerRuntime implements the Runtime interface using Docker or Podman.
// It auto-detects which container engine is available.
type DockerRuntime struct {
	// Command is the container command to use (docker or podman)
	Command string

	// ContainerPrefix is prepended to sandbox names to form container names
	ContainerPrefix string

	// UseRootless indicates whether to use rootless mode
	UseRootless bool

	// StagingDir is the directory for staging generated files
	StagingDir string

	// GeneratedFileMounter handles staging of generated files
	GeneratedFileMounter
}

// NewDockerRuntime creates a new Docker/Podman runtime.
// It auto-detects which command is available.
func NewDockerRuntime(containerPrefix string) (*DockerRuntime, error) {
	// Try podman first (preferred for rootless)
	if _, err := exec.LookPath("podman"); err == nil {
		return &DockerRuntime{
			Command:         "podman",
			ContainerPrefix: containerPrefix,
			UseRootless:     true,
		}, nil
	}

	// Fall back to docker
	if _, err := exec.LookPath("docker"); err == nil {
		return &DockerRuntime{
			Command:         "docker",
			ContainerPrefix: containerPrefix,
			UseRootless:     false,
		}, nil
	}

	return nil, fmt.Errorf("neither podman nor docker found in PATH")
}

// containerName returns the full container name for a sandbox
func (r *DockerRuntime) containerName(sandboxName string) string {
	return r.ContainerPrefix + sandboxName
}

// Name returns the runtime identifier
func (r *DockerRuntime) Name() string {
	return r.Command
}

// runCmd executes a docker/podman command
func (r *DockerRuntime) runCmd(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.Command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s failed: %s: %w", r.Command, args[0], stderr.String(), err)
	}

	return stdout.String(), nil
}

// Create creates a new container from a NixOS image
func (r *DockerRuntime) Create(ctx context.Context, opts CreateOptions) error {
	containerName := r.containerName(opts.Name)
	logging.Debug("creating container", "name", containerName, "runtime", r.Command)

	args := []string{"create", "--name", containerName}

	// Add bind mounts
	for hostPath, containerPath := range opts.BindMounts {
		args = append(args, "-v", fmt.Sprintf("%s:%s", hostPath, containerPath))
	}

	// Add port forwards
	for hostPort, containerPort := range opts.ForwardPorts {
		args = append(args, "-p", fmt.Sprintf("127.0.0.1:%d:%d", hostPort, containerPort))
	}

	// Add extra args
	args = append(args, opts.ExtraArgs...)

	// Use a NixOS-based image - this will need to be built/configured
	// For now, use nixos/nix as a base
	args = append(args, "nixos/nix", "sleep", "infinity")

	_, err := r.runCmd(ctx, args...)
	if err != nil {
		return err
	}

	if opts.Start {
		return r.Start(ctx, opts.Name)
	}

	return nil
}

// Start starts an existing container
func (r *DockerRuntime) Start(ctx context.Context, name string) error {
	containerName := r.containerName(name)
	logging.Debug("starting container", "container", containerName)

	_, err := r.runCmd(ctx, "start", containerName)
	return err
}

// Stop stops a running container
func (r *DockerRuntime) Stop(ctx context.Context, name string) error {
	containerName := r.containerName(name)
	logging.Debug("stopping container", "container", containerName)

	_, err := r.runCmd(ctx, "stop", containerName)
	return err
}

// Destroy stops and removes a container
func (r *DockerRuntime) Destroy(ctx context.Context, name string) error {
	containerName := r.containerName(name)
	logging.Debug("destroying container", "container", containerName)

	// Stop first (ignore errors if already stopped)
	_, _ = r.runCmd(ctx, "stop", containerName)

	// Remove container
	_, err := r.runCmd(ctx, "rm", "-f", containerName)
	if err != nil {
		// Ignore "no such container" errors
		if strings.Contains(err.Error(), "No such container") ||
			strings.Contains(err.Error(), "no such container") {
			return nil
		}
	}

	return err
}

// IsRunning checks if a container is currently running
func (r *DockerRuntime) IsRunning(ctx context.Context, name string) (bool, error) {
	containerName := r.containerName(name)

	output, err := r.runCmd(ctx, "inspect", "-f", "{{.State.Running}}", containerName)
	if err != nil {
		return false, nil // Container doesn't exist
	}

	return strings.TrimSpace(output) == "true", nil
}

// dockerInspect holds the relevant fields from docker inspect
type dockerInspect struct {
	State struct {
		Status    string `json:"Status"`
		Running   bool   `json:"Running"`
		StartedAt string `json:"StartedAt"`
	} `json:"State"`
	NetworkSettings struct {
		IPAddress string `json:"IPAddress"`
	} `json:"NetworkSettings"`
}

// Status returns detailed status of a container
func (r *DockerRuntime) Status(ctx context.Context, name string) (*ContainerInfo, error) {
	containerName := r.containerName(name)

	info := &ContainerInfo{
		Name:   name,
		Status: StatusNotFound,
	}

	output, err := r.runCmd(ctx, "inspect", containerName)
	if err != nil {
		return info, nil
	}

	var inspects []dockerInspect
	if err := json.Unmarshal([]byte(output), &inspects); err != nil {
		return info, nil
	}

	if len(inspects) == 0 {
		return info, nil
	}

	inspect := inspects[0]
	switch inspect.State.Status {
	case "running":
		info.Status = StatusRunning
	case "exited", "stopped", "created":
		info.Status = StatusStopped
	default:
		info.Status = StatusUnknown
	}

	info.StartedAt = inspect.State.StartedAt
	info.IPAddress = inspect.NetworkSettings.IPAddress

	return info, nil
}

// Exec executes a command inside a container
func (r *DockerRuntime) Exec(ctx context.Context, name string, command []string, opts ExecOptions) (*ExecResult, error) {
	containerName := r.containerName(name)

	args := []string{"exec"}

	if opts.Interactive {
		args = append(args, "-it")
	}

	if opts.User != "" {
		args = append(args, "-u", opts.User)
	}

	if opts.WorkingDir != "" {
		args = append(args, "-w", opts.WorkingDir)
	}

	for _, env := range opts.Env {
		args = append(args, "-e", env)
	}

	args = append(args, containerName)
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, r.Command, args...)

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
func (r *DockerRuntime) ExecInteractive(ctx context.Context, name string, command []string, opts ExecOptions) error {
	containerName := r.containerName(name)

	cmdPath, err := exec.LookPath(r.Command)
	if err != nil {
		return fmt.Errorf("%s not found: %w", r.Command, err)
	}

	args := []string{r.Command, "exec", "-it"}

	if opts.User != "" {
		args = append(args, "-u", opts.User)
	}

	if opts.WorkingDir != "" {
		args = append(args, "-w", opts.WorkingDir)
	}

	args = append(args, containerName)
	args = append(args, command...)

	return syscall.Exec(cmdPath, args, os.Environ())
}

// List returns all containers managed by this runtime
func (r *DockerRuntime) List(ctx context.Context) ([]*ContainerInfo, error) {
	output, err := r.runCmd(ctx, "ps", "-a", "--format", "{{.Names}}", "--filter", fmt.Sprintf("name=%s", r.ContainerPrefix))
	if err != nil {
		return nil, err
	}

	var containers []*ContainerInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, name := range lines {
		if name == "" {
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

// ContainerInfo returns information about the container environment.
func (r *DockerRuntime) ContainerInfo() SandboxContainerInfo {
	return DefaultContainerInfo()
}

// Ensure DockerRuntime implements Runtime and GeneratedFileRuntime
var _ Runtime = (*DockerRuntime)(nil)
var _ GeneratedFileRuntime = (*DockerRuntime)(nil)
