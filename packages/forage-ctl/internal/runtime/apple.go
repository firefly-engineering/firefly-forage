// Package runtime provides container runtime implementations.
// This file implements the Apple Container backend for macOS.
//
// Apple Container (github.com/apple/containerization) uses Apple's
// Virtualization.framework to run Linux containers in lightweight VMs.
// This provides better isolation than Docker Desktop on macOS while
// maintaining good performance.
//
// Prerequisites:
// - macOS 13+ (Ventura or later)
// - Apple Silicon or Intel with Virtualization support
// - The 'container' CLI tool installed
//
// Installation:
//   brew install apple/containerization/container
//
// Note: This backend requires the nix store to be available in the VM.
// Options include:
// 1. Use Determinate Nix installer (recommended for macOS)
// 2. Use nix-darwin with store sharing
// 3. Mount the nix store from the host (requires VM configuration)

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"
	"syscall"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
)

// AppleRuntime implements the Runtime interface using Apple Container.
type AppleRuntime struct {
	// ContainerPrefix is prepended to sandbox names to form container names
	ContainerPrefix string

	// BinaryPath is the path to the container CLI
	BinaryPath string

	// SandboxesDir is the directory containing sandbox metadata files
	// Used to resolve container names from metadata
	SandboxesDir string
}

// NewAppleRuntime creates a new Apple Container runtime.
func NewAppleRuntime(containerPrefix, sandboxesDir string) (*AppleRuntime, error) {
	// Apple Container only works on macOS
	if goruntime.GOOS != "darwin" {
		return nil, fmt.Errorf("Apple Container is only available on macOS")
	}

	// Look for the container binary
	binaryPath, err := exec.LookPath("container")
	if err != nil {
		return nil, fmt.Errorf("Apple Container CLI not found. Install with: brew install apple/containerization/container")
	}

	return &AppleRuntime{
		ContainerPrefix: containerPrefix,
		BinaryPath:      binaryPath,
		SandboxesDir:    sandboxesDir,
	}, nil
}

// containerName returns the full container name for a sandbox.
// It loads metadata to use the short container name if available,
// falling back to the legacy prefix+name format.
func (r *AppleRuntime) containerName(sandboxName string) string {
	if r.SandboxesDir != "" {
		if meta, err := config.LoadSandboxMetadata(r.SandboxesDir, sandboxName); err == nil {
			return meta.ResolvedContainerName()
		}
	}
	return r.ContainerPrefix + sandboxName
}

// Name returns the runtime identifier
func (r *AppleRuntime) Name() string {
	return "apple"
}

// runCmd executes an Apple Container command
func (r *AppleRuntime) runCmd(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.BinaryPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("container %s failed: %s: %w", args[0], stderr.String(), err)
	}

	return stdout.String(), nil
}

// Create creates a new container
func (r *AppleRuntime) Create(ctx context.Context, opts CreateOptions) error {
	containerName := r.containerName(opts.Name)
	logging.Debug("creating container", "name", containerName, "runtime", "apple")

	// Apple Container uses 'container run' with various options
	args := []string{"run", "--name", containerName, "--detach"}

	// Add bind mounts
	for hostPath, containerPath := range opts.BindMounts {
		args = append(args, "--mount", fmt.Sprintf("type=bind,source=%s,target=%s", hostPath, containerPath))
	}

	// Add port forwards
	for hostPort, containerPort := range opts.ForwardPorts {
		args = append(args, "--publish", fmt.Sprintf("127.0.0.1:%d:%d", hostPort, containerPort))
	}

	// Add extra args
	args = append(args, opts.ExtraArgs...)

	// Use a NixOS-compatible image
	// Apple Container can pull OCI images
	args = append(args, "nixos/nix:latest")

	// Keep the container running
	args = append(args, "sleep", "infinity")

	_, err := r.runCmd(ctx, args...)
	if err != nil {
		return err
	}

	// Apple Container's 'run --detach' starts immediately, so no separate Start needed
	return nil
}

// Start starts an existing container
func (r *AppleRuntime) Start(ctx context.Context, name string) error {
	containerName := r.containerName(name)
	logging.Debug("starting container", "container", containerName)

	_, err := r.runCmd(ctx, "start", containerName)
	return err
}

// Stop stops a running container
func (r *AppleRuntime) Stop(ctx context.Context, name string) error {
	containerName := r.containerName(name)
	logging.Debug("stopping container", "container", containerName)

	_, err := r.runCmd(ctx, "stop", containerName)
	return err
}

// Destroy stops and removes a container
func (r *AppleRuntime) Destroy(ctx context.Context, name string) error {
	containerName := r.containerName(name)
	logging.Debug("destroying container", "container", containerName)

	// Stop first (ignore errors if already stopped)
	_, _ = r.runCmd(ctx, "stop", containerName)

	// Remove container
	_, err := r.runCmd(ctx, "rm", "-f", containerName)
	if err != nil {
		// Ignore "not found" errors
		if strings.Contains(err.Error(), "not found") ||
			strings.Contains(err.Error(), "No such container") {
			return nil
		}
	}

	return err
}

// IsRunning checks if a container is currently running
func (r *AppleRuntime) IsRunning(ctx context.Context, name string) (bool, error) {
	containerName := r.containerName(name)

	output, err := r.runCmd(ctx, "inspect", containerName, "--format", "{{.State.Running}}")
	if err != nil {
		return false, nil // Container doesn't exist
	}

	return strings.TrimSpace(output) == "true", nil
}

// appleInspect holds the relevant fields from container inspect
type appleInspect struct {
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
func (r *AppleRuntime) Status(ctx context.Context, name string) (*ContainerInfo, error) {
	containerName := r.containerName(name)

	info := &ContainerInfo{
		Name:   name,
		Status: StatusNotFound,
	}

	output, err := r.runCmd(ctx, "inspect", containerName)
	if err != nil {
		return info, nil
	}

	var inspects []appleInspect
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
func (r *AppleRuntime) Exec(ctx context.Context, name string, command []string, opts ExecOptions) (*ExecResult, error) {
	containerName := r.containerName(name)

	args := []string{"exec"}

	if opts.Interactive {
		args = append(args, "-it")
	}

	if opts.User != "" {
		args = append(args, "--user", opts.User)
	}

	if opts.WorkingDir != "" {
		args = append(args, "--workdir", opts.WorkingDir)
	}

	for _, env := range opts.Env {
		args = append(args, "--env", env)
	}

	args = append(args, containerName)
	args = append(args, command...)

	cmd := exec.CommandContext(ctx, r.BinaryPath, args...)

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
func (r *AppleRuntime) ExecInteractive(ctx context.Context, name string, command []string, opts ExecOptions) error {
	containerName := r.containerName(name)

	args := []string{r.BinaryPath, "exec", "-it"}

	if opts.User != "" {
		args = append(args, "--user", opts.User)
	}

	if opts.WorkingDir != "" {
		args = append(args, "--workdir", opts.WorkingDir)
	}

	args = append(args, containerName)
	args = append(args, command...)

	return syscall.Exec(r.BinaryPath, args, os.Environ())
}

// List returns all containers managed by this runtime
func (r *AppleRuntime) List(ctx context.Context) ([]*ContainerInfo, error) {
	// Build reverse mapping: container name → sandbox name from metadata
	reverseMap := buildContainerReverseMap(r.SandboxesDir)

	// List all containers
	output, err := r.runCmd(ctx, "ps", "-a", "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}

	var containers []*ContainerInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, name := range lines {
		if name == "" {
			continue
		}

		var sandboxName string
		if sn, ok := reverseMap[name]; ok {
			sandboxName = sn
		} else if strings.HasPrefix(name, r.ContainerPrefix) {
			// Legacy fallback: strip prefix
			sandboxName = strings.TrimPrefix(name, r.ContainerPrefix)
		} else {
			continue // Not a forage container
		}

		info, _ := r.Status(ctx, sandboxName)
		if info != nil {
			containers = append(containers, info)
		}
	}

	return containers, nil
}

// Ensure AppleRuntime implements Runtime
var _ Runtime = (*AppleRuntime)(nil)
