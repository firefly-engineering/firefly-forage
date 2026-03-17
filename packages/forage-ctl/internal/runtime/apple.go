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
//
//	brew install apple/containerization/container
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
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	goruntime "runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	shellquote "github.com/kballard/go-shellquote"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/system"
)

// defaultAppleImage is the default OCI image for Apple Container sandboxes.
const defaultAppleImage = "nixos/nix:latest"

// AppleRuntime implements the Runtime interface using Apple Container.
type AppleRuntime struct {
	// ContainerPrefix is prepended to sandbox names to form container names
	ContainerPrefix string

	// BinaryPath is the path to the container CLI
	BinaryPath string

	// SandboxesDir is the directory containing sandbox metadata files
	// Used to resolve container names from metadata
	SandboxesDir string

	// Image overrides the default OCI image (defaultAppleImage).
	// When empty, defaultAppleImage is used.
	Image string

	// GeneratedFileMounter handles staging of generated files
	GeneratedFileMounter
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

	rt := &AppleRuntime{
		ContainerPrefix: containerPrefix,
		BinaryPath:      binaryPath,
		SandboxesDir:    sandboxesDir,
	}

	// Run preflight checks (warnings only — don't prevent creation)
	rt.preflight()

	return rt, nil
}

// preflight validates prerequisites and logs actionable warnings.
// It does not return errors because missing prerequisites may be
// resolved before the first container creation.
func (r *AppleRuntime) preflight() {
	// Check CLI version / health
	output, err := r.runCmd(context.Background(), "version")
	if err != nil {
		logging.Warn("Apple Container CLI may not be functional", "error", err)
	} else {
		logging.Debug("Apple Container CLI version", "output", strings.TrimSpace(output))
	}

	// Check Nix store is accessible on the host
	if _, err := os.Stat("/nix/store"); err != nil {
		logging.Warn("Nix store not found at /nix/store — containers will not have access to Nix packages. Install Nix: https://nixos.org/download")
	}

	// Check nix-daemon socket
	if _, err := os.Stat("/nix/var/nix/daemon-socket/socket"); err != nil {
		logging.Warn("Nix daemon socket not found — container builds may fail. Ensure nix-daemon is running")
	}
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

// containerCmdError wraps an Apple Container CLI error with the exit code
// so callers can branch on exit codes instead of fragile string matching.
type containerCmdError struct {
	Subcommand string
	ExitCode   int
	Stderr     string
	Err        error
}

func (e *containerCmdError) Error() string {
	return fmt.Sprintf("container %s failed: %s: %v", e.Subcommand, e.Stderr, e.Err)
}

func (e *containerCmdError) Unwrap() error { return e.Err }

// isNotFound returns true when the CLI indicated the target resource does not exist.
// It checks the exit code first (non-zero) and falls back to stderr keywords
// for CLIs that don't use distinct exit codes.
func isContainerNotFound(err error) bool {
	var cmdErr *containerCmdError
	if !errors.As(err, &cmdErr) {
		return false
	}
	// Any non-zero exit + stderr containing a "not found" variant
	if cmdErr.ExitCode == 0 {
		return false
	}
	lower := strings.ToLower(cmdErr.Stderr)
	return strings.Contains(lower, "not found") ||
		strings.Contains(lower, "no such container") ||
		strings.Contains(lower, "does not exist")
}

// runCmd executes an Apple Container command
func (r *AppleRuntime) runCmd(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.BinaryPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return "", &containerCmdError{
			Subcommand: args[0],
			ExitCode:   exitCode,
			Stderr:     stderr.String(),
			Err:        err,
		}
	}

	return stdout.String(), nil
}

// Create creates a new container.
// Apple Container's 'run --detach' creates and starts in one step.
// When opts.Start is false, we use 'create' instead.
func (r *AppleRuntime) Create(ctx context.Context, opts CreateOptions) error {
	containerName := r.containerName(opts.Name)
	logging.Debug("creating container", "name", containerName, "runtime", "apple")

	var subcommand string
	var args []string
	if opts.Start {
		subcommand = "run"
		args = []string{subcommand, "--name", containerName, "--detach"}
	} else {
		subcommand = "create"
		args = []string{subcommand, "--name", containerName}
	}

	// Mount the host Nix store and daemon socket into the VM so the
	// container can use the host's Nix daemon for builds.
	platformCfg := DetectPlatformMountConfig()
	nixMounts := NewStandardMounts("", "", "")
	if platformCfg.UseBindMount {
		// Nix store (read-only)
		args = append(args, "--mount", fmt.Sprintf("type=bind,source=%s,target=%s,readonly",
			platformCfg.NixStorePath, platformCfg.NixStorePath))
		// Nix daemon socket (read-write so the container can issue build requests)
		if _, err := os.Stat(platformCfg.NixDaemonSocketPath); err == nil {
			args = append(args, "--mount", fmt.Sprintf("type=bind,source=%s,target=%s",
				platformCfg.NixDaemonSocketPath, platformCfg.NixDaemonSocketPath))
		}
	}
	_ = nixMounts // mounts handled inline above; struct kept for future use

	// Add bind mounts
	for hostPath, containerPath := range opts.BindMounts {
		args = append(args, "--mount", fmt.Sprintf("type=bind,source=%s,target=%s", hostPath, containerPath))
	}

	// Add port forwards
	for hostPort, containerPort := range opts.ForwardPorts {
		args = append(args, "--publish", fmt.Sprintf("127.0.0.1:%d:%d", hostPort, containerPort))
	}

	// Network isolation
	switch opts.NetworkMode {
	case "none":
		args = append(args, "--network=none")
	case "restricted":
		// Start with networking enabled; iptables rules are injected post-start
		// via applyRestrictedNetwork
	default:
		// "full" or empty: default networking
	}

	// Add labels for orphan detection
	args = append(args,
		"--label", "forage.sandbox-name="+opts.Name,
		"--label", "forage.runtime=apple",
		"--label", "forage.container-name="+containerName,
	)

	// Apply resource limits via Apple Container CLI flags
	if opts.CPUQuota != "" {
		// Apple Container uses --cpus (float, e.g. "2.0" for 2 cores).
		// Convert from percentage format (e.g. "200%") to float.
		cpus := cpuQuotaToFloat(opts.CPUQuota)
		if cpus != "" {
			args = append(args, "--cpus", cpus)
		}
	}
	if opts.MemoryMax != "" {
		args = append(args, "--memory", opts.MemoryMax)
	}

	// Add environment variables
	for k, v := range opts.EnvVars {
		args = append(args, "-e", k+"="+v)
	}

	// Add extra args
	args = append(args, opts.ExtraArgs...)

	// Use a NixOS-compatible image.
	// The nixos/nix image has binaries in the nix store, not at standard paths.
	// Use /bin/sh -c to ensure the nix profile PATH is sourced.
	args = append(args, r.containerImage())
	args = append(args, "/bin/sh", "-c", "exec sleep infinity")

	_, err := r.runCmd(ctx, args...)
	if err != nil {
		return err
	}

	// Post-start tasks
	if opts.Start {
		// Validate Nix store is accessible inside the container
		if valErr := r.validateNixStore(ctx, opts.Name); valErr != nil {
			logging.Warn("Nix store may not be accessible in container", "error", valErr)
		}

		// Apply iptables rules for restricted network mode
		if opts.NetworkMode == "restricted" {
			if netErr := r.applyRestrictedNetwork(ctx, opts.Name, opts.AllowedHosts); netErr != nil {
				return fmt.Errorf("failed to apply network restrictions: %w", netErr)
			}
		}
	}

	return nil
}

// applyRestrictedNetwork injects iptables rules inside the container to restrict
// outbound traffic to only the allowed hosts. This is used for the "restricted"
// network mode where the container starts with full networking and then gets
// filtered. The iptables approach works inside any Linux container without
// requiring NixOS or nftables.
func (r *AppleRuntime) applyRestrictedNetwork(ctx context.Context, sandboxName string, allowedHosts []string) error {
	logging.Debug("applying restricted network rules", "sandbox", sandboxName, "allowedHosts", allowedHosts)

	// Resolve allowed hosts to IPs
	resolved, err := net.LookupIP("localhost") // Warm up resolver
	_ = resolved
	_ = err

	// Build the iptables rules script
	var script strings.Builder
	script.WriteString("set -e\n")
	// Default policy: drop outbound
	script.WriteString("iptables -P OUTPUT DROP 2>/dev/null || true\n")
	// Allow loopback
	script.WriteString("iptables -A OUTPUT -o lo -j ACCEPT 2>/dev/null || true\n")
	// Allow established/related
	script.WriteString("iptables -A OUTPUT -m state --state ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true\n")

	// Allow each host
	for _, host := range allowedHosts {
		ips, lookupErr := net.LookupIP(host)
		if lookupErr != nil {
			logging.Warn("failed to resolve host for network restriction", "host", host, "error", lookupErr)
			continue
		}
		for _, ip := range ips {
			if ip.To4() != nil {
				fmt.Fprintf(&script, "iptables -A OUTPUT -d %s -j ACCEPT 2>/dev/null || true\n", ip.String())
			}
		}
	}

	// Reject remaining with ICMP unreachable (better than silent drop)
	script.WriteString("iptables -A OUTPUT -j REJECT 2>/dev/null || true\n")

	result, execErr := r.Exec(ctx, sandboxName, []string{"sh", "-c", script.String()}, ExecOptions{User: "root"})
	if execErr != nil {
		return fmt.Errorf("failed to apply iptables rules: %w", execErr)
	}
	if result.ExitCode != 0 {
		logging.Warn("iptables rules may not have applied cleanly", "stderr", result.Stderr)
	}

	logging.Debug("restricted network rules applied", "sandbox", sandboxName)
	return nil
}

// validateNixStore checks that the Nix store is accessible inside the container.
func (r *AppleRuntime) validateNixStore(ctx context.Context, sandboxName string) error {
	result, err := r.Exec(ctx, sandboxName, []string{"ls", "/nix/store"}, ExecOptions{})
	if err != nil {
		return fmt.Errorf("failed to validate nix store: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("nix store not accessible (exit %d): %s", result.ExitCode, result.Stderr)
	}
	return nil
}

// containerImage returns the OCI image to use for containers.
func (r *AppleRuntime) containerImage() string {
	if r.Image != "" {
		return r.Image
	}
	return defaultAppleImage
}

// cpuQuotaToFloat converts a CPU quota string (e.g. "200%" for 2 cores)
// to a float string (e.g. "2.0") suitable for Apple Container's --cpus flag.
func cpuQuotaToFloat(quota string) string {
	s := strings.TrimSuffix(quota, "%")
	pct, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return ""
	}
	return strconv.FormatFloat(pct/100, 'f', 1, 64)
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

	// Remove container (Apple CLI uses 'rm' or 'delete', no -f flag)
	_, err := r.runCmd(ctx, "rm", containerName)
	if err != nil && isContainerNotFound(err) {
		return nil
	}

	return err
}

// IsRunning checks if a container is currently running
func (r *AppleRuntime) IsRunning(ctx context.Context, name string) (bool, error) {
	containerName := r.containerName(name)

	output, err := r.runCmd(ctx, "inspect", containerName)
	if err != nil {
		return false, nil // Container doesn't exist
	}

	var inspects []appleInspect
	if err := json.Unmarshal([]byte(output), &inspects); err != nil {
		return false, nil
	}

	if len(inspects) == 0 {
		return false, nil
	}

	return inspects[0].Status == "running", nil
}

// appleInspect holds the relevant fields from Apple Container's inspect JSON.
// The Apple CLI returns a different schema than Docker:
//
//	[{"status": "running", "startedDate": 12345.67,
//	  "configuration": {"id": "name", ...},
//	  "networks": [{"ipv4Address": "192.168.64.2/24", ...}]}]
type appleInspect struct {
	Status        string  `json:"status"`
	StartedDate   float64 `json:"startedDate"`
	Configuration struct {
		ID string `json:"id"`
	} `json:"configuration"`
	Networks []struct {
		IPv4Address string `json:"ipv4Address"`
		IPv6Address string `json:"ipv6Address"`
		Network     string `json:"network"`
	} `json:"networks"`
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
	switch inspect.Status {
	case "running":
		info.Status = StatusRunning
	case "exited", "stopped", "created":
		info.Status = StatusStopped
	default:
		info.Status = StatusUnknown
	}

	// Extract IP address from first network, stripping CIDR suffix
	if len(inspect.Networks) > 0 {
		ip := inspect.Networks[0].IPv4Address
		if idx := strings.IndexByte(ip, '/'); idx >= 0 {
			ip = ip[:idx]
		}
		info.IPAddress = ip
	}

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
		args = append(args, "-u", opts.User)
	}

	if opts.WorkingDir != "" {
		args = append(args, "-w", opts.WorkingDir)
	}

	for _, env := range opts.Env {
		args = append(args, "-e", env)
	}

	args = append(args, containerName)

	// Wrap in /bin/sh so the container's PATH (nix profile paths) is used.
	// Without this, commands like "ls" won't be found since nix-based
	// containers don't place binaries in standard locations.
	args = append(args, "/bin/sh", "-c", shellquote.Join(command...))

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
		args = append(args, "-u", opts.User)
	}

	if opts.WorkingDir != "" {
		args = append(args, "-w", opts.WorkingDir)
	}

	args = append(args, containerName)
	args = append(args, "/bin/sh", "-c", shellquote.Join(command...))

	return syscall.Exec(r.BinaryPath, args, system.SafeEnviron())
}

// appleListEntry holds the fields from Apple Container's list --format json.
type appleListEntry struct {
	Status        string `json:"status"`
	Configuration struct {
		ID string `json:"id"`
	} `json:"configuration"`
}

// List returns all containers managed by this runtime
func (r *AppleRuntime) List(ctx context.Context) ([]*ContainerInfo, error) {
	// Build reverse mapping: container name → sandbox name from metadata
	reverseMap := buildContainerReverseMap(r.SandboxesDir)

	// Apple CLI uses 'list --all --format json' (not Docker's ps --format template)
	output, err := r.runCmd(ctx, "list", "--all", "--format", "json")
	if err != nil {
		return nil, err
	}

	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	var entries []appleListEntry
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		return nil, fmt.Errorf("failed to parse container list: %w", err)
	}

	var containers []*ContainerInfo
	for _, entry := range entries {
		name := entry.Configuration.ID
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

// GracefulStop uses Apple Container's stop command.
func (r *AppleRuntime) GracefulStop(ctx context.Context, name string, timeout time.Duration) error {
	containerName := r.containerName(name)
	logging.Debug("graceful stop", "container", containerName, "timeout", timeout)

	// Apple Container's stop doesn't have a timeout flag,
	// so we use context timeout instead
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := r.runCmd(ctx, "stop", containerName)
	return err
}

// ContainerInfo returns information about the container environment.
func (r *AppleRuntime) ContainerInfo() SandboxContainerInfo {
	return DefaultContainerInfo()
}

// ViewLogs streams container logs via 'container logs'.
func (r *AppleRuntime) ViewLogs(ctx context.Context, name string, follow bool, lines int) error {
	containerName := r.containerName(name)

	args := []string{r.BinaryPath, "logs", containerName, "--tail", fmt.Sprintf("%d", lines)}
	if follow {
		args = append(args, "--follow")
	}

	return syscall.Exec(r.BinaryPath, args, system.SafeEnviron())
}

// Capabilities returns the capabilities of Apple Container runtime.
// Apple Container supports resource limits (CPU, memory) via --cpus and --memory.
func (r *AppleRuntime) Capabilities() Capabilities {
	return Capabilities{
		NixOSConfig:      false,
		NetworkIsolation: true,
		EphemeralRoot:    true,
		SSHAccess:        false,
		GeneratedFiles:   true,
		ResourceLimits:   true,
		GracefulShutdown: true,
	}
}

// Ensure AppleRuntime implements Runtime, GeneratedFileRuntime, CapableRuntime, GracefulStopper, and LogViewer
var _ Runtime = (*AppleRuntime)(nil)
var _ GeneratedFileRuntime = (*AppleRuntime)(nil)
var _ CapableRuntime = (*AppleRuntime)(nil)
var _ GracefulStopper = (*AppleRuntime)(nil)
var _ LogViewer = (*AppleRuntime)(nil)
