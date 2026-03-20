package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/generator"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/ssh"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/system"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/telemetry"
)

// NspawnRuntime implements the Runtime interface using systemd-nspawn
// for NixOS systems. Container lifecycle (install, start, destroy) is
// managed directly via systemd and symlinks into /etc/systemd-mutable/system.
type NspawnRuntime struct {
	// ContainerPrefix is prepended to sandbox names to form container names
	ContainerPrefix string

	// SandboxesDir is the directory containing sandbox metadata files
	// Used for looking up SSH ports from persisted metadata
	SandboxesDir string

	// NixpkgsPath is the Nix store path to nixpkgs source.
	// Used for nix-build of container configurations.
	NixpkgsPath string

	// GeneratedFileMounter handles staging of generated files
	GeneratedFileMounter
}

// NewNspawnRuntime creates a new nspawn runtime with the given configuration
func NewNspawnRuntime(containerPrefix, sandboxesDir, nixpkgsPath string) *NspawnRuntime {
	return &NspawnRuntime{
		ContainerPrefix: containerPrefix,
		SandboxesDir:    sandboxesDir,
		NixpkgsPath:     nixpkgsPath,
		GeneratedFileMounter: GeneratedFileMounter{
			StagingDir: sandboxesDir,
		},
	}
}

// containerName returns the full container name for a sandbox.
// It loads metadata to use the short container name if available,
// falling back to the legacy prefix+name format.
func (r *NspawnRuntime) containerName(sandboxName string) string {
	if r.SandboxesDir != "" {
		if meta, err := config.LoadSandboxMetadata(r.SandboxesDir, sandboxName); err == nil {
			return meta.ResolvedContainerName()
		}
	}
	return r.ContainerPrefix + sandboxName
}

// Name returns the runtime identifier
func (r *NspawnRuntime) Name() string {
	return "nspawn"
}

// Create creates a new container by building the Nix config and installing
// the resulting systemd units directly via symlinks into /etc/systemd-mutable/system.
func (r *NspawnRuntime) Create(ctx context.Context, opts CreateOptions) error {
	ctx, span := telemetry.Start(ctx, "nspawn.create")
	defer span.End()

	logging.Debug("creating container", "name", opts.Name, "config", opts.ConfigPath)

	// If the config path is a nix store path (pre-built /etc), install directly
	if strings.HasPrefix(opts.ConfigPath, "/nix/store/") {
		return r.CreateFromEtc(ctx, opts.ConfigPath, opts.Start)
	}

	// Otherwise, build the config first using our eval-config.nix
	evalConfigPath := filepath.Join(r.SandboxesDir, opts.Name+".eval-config.nix")
	if err := os.WriteFile(evalConfigPath, []byte(generator.EvalConfigNix), 0644); err != nil {
		return fmt.Errorf("failed to write eval-config.nix: %w", err)
	}

	etcPath, err := r.BuildOuterEtc(ctx, opts.ConfigPath, evalConfigPath)
	if err != nil {
		return fmt.Errorf("nix-build container config failed: %w", err)
	}

	return installContainer(ctx, etcPath, opts.Start)
}

// Start starts an existing container. Uses the fastest available path:
// 1. Cached etc path from metadata → CreateFromEtc (~1s, no eval at all)
// 2. Outer .nix + our eval-config → BuildOuterEtc + CreateFromEtc (~2s)
// 3. Full .nix through nix-build + install (fallback, ~17s)
func (r *NspawnRuntime) Start(ctx context.Context, name string) error {
	ctx, span := telemetry.Start(ctx, "nspawn.start")
	defer span.End()

	if r.SandboxesDir == "" {
		return r.startFallback(ctx, name)
	}

	// Fast path 1: use cached etc path from metadata (no Nix eval at all)
	if meta, err := config.LoadSandboxMetadata(r.SandboxesDir, name); err == nil && meta.CachedEtcPath != "" {
		if _, err := os.Stat(meta.CachedEtcPath); err == nil {
			logging.Debug("starting container via cached etc", "name", name, "etcPath", meta.CachedEtcPath)
			if err := r.CreateFromEtc(ctx, meta.CachedEtcPath, true); err == nil {
				return nil
			}
			logging.Warn("cached etc start failed, trying outer config", "name", name)
		}
	}

	// Fast path 2: build outer etc using our stripped eval-config
	outerPath := r.SandboxesDir + "/" + name + ".outer.nix"
	if _, err := os.Stat(outerPath); err == nil {
		etcPath, err := r.buildAndCacheOuterEtc(ctx, name, outerPath)
		if err == nil {
			logging.Debug("starting container via freshly built etc", "name", name, "etcPath", etcPath)
			if startErr := r.CreateFromEtc(ctx, etcPath, true); startErr == nil {
				return nil
			}
			logging.Warn("outer etc start failed, falling back to full rebuild", "name", name)
		} else {
			logging.Warn("outer etc build failed, falling back to full rebuild", "name", name, "error", err)
		}
	}

	// Slow path: rebuild from full .nix config through nix-build + install
	return r.startFallback(ctx, name)
}

// startFallback starts a container via the full .nix config through nix-build + install.
func (r *NspawnRuntime) startFallback(ctx context.Context, name string) error {
	configPath := r.SandboxesDir + "/" + name + ".nix"
	logging.Debug("starting container via nix-build", "name", name, "config", configPath)
	return r.Create(ctx, CreateOptions{
		Name:       name,
		ConfigPath: configPath,
		Start:      true,
	})
}

// buildAndCacheOuterEtc writes the eval-config.nix to the sandbox staging dir,
// builds outer etc, and saves the etc path in metadata.
func (r *NspawnRuntime) buildAndCacheOuterEtc(ctx context.Context, name, outerPath string) (string, error) {
	evalConfigPath := r.SandboxesDir + "/" + name + ".eval-config.nix"
	if err := os.WriteFile(evalConfigPath, []byte(generator.EvalConfigNix), 0644); err != nil {
		return "", fmt.Errorf("failed to write eval-config.nix: %w", err)
	}

	etcPath, err := r.BuildOuterEtc(ctx, outerPath, evalConfigPath)
	if err != nil {
		return "", err
	}

	// Save cached etc path in metadata for future fast restarts
	if meta, loadErr := config.LoadSandboxMetadata(r.SandboxesDir, name); loadErr == nil {
		meta.CachedEtcPath = etcPath
		_ = config.SaveSandboxMetadata(r.SandboxesDir, meta)
	}

	return etcPath, nil
}

// BuildInnerSystem builds the inner NixOS system from a config file and returns
// its store path. Uses nix-build '<nixpkgs/nixos>' -A system.build.toplevel.
func (r *NspawnRuntime) BuildInnerSystem(ctx context.Context, configPath string) (string, error) {
	ctx, span := telemetry.Start(ctx, "nspawn.build-inner-system")
	defer span.End()

	logging.Info("nixcache: building inner system", "config", configPath)

	nixpkgsExpr := "<nixpkgs/nixos>"
	if r.NixpkgsPath != "" {
		nixpkgsExpr = r.NixpkgsPath + "/nixos"
	}

	args := []string{
		"nix-build", nixpkgsExpr,
		"-A", "config.system.build.toplevel",
		"--arg", "configuration", configPath,
		"--no-out-link",
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	tracer := newNixOutputTracer(span)
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr, tracer)

	span.AddEvent("subprocess.start")
	err := cmd.Run()
	tracer.Flush()
	if err != nil {
		// Include stderr in error message so fallback log reveals the Nix evaluation error
		errMsg := strings.TrimSpace(stderr.String())
		if len(errMsg) > 500 {
			errMsg = errMsg[len(errMsg)-500:]
		}
		return "", fmt.Errorf("nix-build inner system failed: %w\nstderr: %s", err, errMsg)
	}

	storePath := strings.TrimSpace(stdout.String())
	if storePath == "" {
		return "", fmt.Errorf("nix-build produced empty output")
	}

	span.SetAttributes(attribute.String("store.path", storePath))
	logging.Info("nixcache: inner system built", "path", storePath)
	return storePath, nil
}

// BuildOuterEtc builds the outer container /etc from an outer config file using
// our stripped eval-config.nix (minimal module set). This
// evaluates in ~0.5s instead of ~13s because no inner NixOS system is evaluated.
// Returns the /nix/store path of the built etc derivation.
func (r *NspawnRuntime) BuildOuterEtc(ctx context.Context, outerConfigPath, evalConfigPath string) (string, error) {
	ctx, span := telemetry.Start(ctx, "nspawn.build-outer-etc")
	defer span.End()

	logging.Info("building outer /etc", "config", outerConfigPath)

	nixosPath := "<nixpkgs/nixos>"
	if r.NixpkgsPath != "" {
		nixosPath = r.NixpkgsPath + "/nixos"
	}

	// Build the etc derivation using our stripped eval-config.nix
	nixExpr := fmt.Sprintf(`
let
  cfg = import ''%s'';
in (import %s {
  nixosPath = %s;
  systemConfig = cfg;
}).config.system.build.etc
`, outerConfigPath, evalConfigPath, nixosPath)

	args := []string{
		"nix-build", "--no-out-link", "-E", nixExpr,
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	tracer := newNixOutputTracer(span)
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr, tracer)

	span.AddEvent("subprocess.start")
	err := cmd.Run()
	tracer.Flush()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if len(errMsg) > 500 {
			errMsg = errMsg[len(errMsg)-500:]
		}
		return "", fmt.Errorf("nix-build outer etc failed: %w\nstderr: %s", err, errMsg)
	}

	etcPath := strings.TrimSpace(stdout.String())
	if etcPath == "" {
		return "", fmt.Errorf("nix-build outer etc produced empty output")
	}

	span.SetAttributes(attribute.String("etc.path", etcPath))
	logging.Info("outer /etc built", "path", etcPath)
	return etcPath, nil
}

// CreateFromEtc creates a container directly from a pre-built /etc store path.
// This installs systemd units and starts the container without any Nix evaluation.
func (r *NspawnRuntime) CreateFromEtc(ctx context.Context, etcPath string, start bool) error {
	ctx, span := telemetry.Start(ctx, "nspawn.create-from-etc")
	defer span.End()

	logging.Debug("creating container from pre-built etc", "etcPath", etcPath)
	span.AddEvent("install.start")

	return installContainer(ctx, etcPath, start)
}

// Stop stops a running container
func (r *NspawnRuntime) Stop(ctx context.Context, name string) error {
	ctx, span := telemetry.Start(ctx, "nspawn.stop")
	defer span.End()

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
	ctx, span := telemetry.Start(ctx, "nspawn.destroy")
	defer span.End()

	containerName := r.containerName(name)
	logging.Debug("destroying container", "container", containerName)

	return destroyContainer(ctx, containerName)
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
	ctx, span := telemetry.Start(ctx, "nspawn.exec",
		telemetry.WithAttr(attribute.String("cmd", strings.Join(command, " "))))
	defer span.End()

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

	return syscall.Exec(machinectlPath, args, system.SafeEnviron())
}

// List returns all containers managed by this runtime
func (r *NspawnRuntime) List(ctx context.Context) ([]*ContainerInfo, error) {
	// Build reverse mapping: container name → sandbox name from metadata
	reverseMap := buildContainerReverseMap(r.SandboxesDir)

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

		var sandboxName string
		if sn, ok := reverseMap[name]; ok {
			sandboxName = sn
		} else if strings.HasPrefix(name, r.ContainerPrefix) {
			// Legacy fallback: strip prefix
			sandboxName = strings.TrimPrefix(name, r.ContainerPrefix)
		} else if sn := readForageJSONSandboxName(ctx, name); sn != "" {
			// Fallback: query /etc/forage.json from running container
			sandboxName = sn
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
	ctx, span := telemetry.Start(ctx, "nspawn.ssh-exec")
	defer span.End()

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

// ContainerInfo returns information about the container environment.
func (r *NspawnRuntime) ContainerInfo() SandboxContainerInfo {
	return DefaultContainerInfo()
}

// forageJSON represents the /etc/forage.json metadata inside a container.
type forageJSON struct {
	SandboxName   string `json:"sandboxName"`
	ContainerName string `json:"containerName"`
	Runtime       string `json:"runtime"`
}

// readForageJSONSandboxName attempts to read /etc/forage.json from a running
// container via machinectl shell. Returns the sandbox name or empty string on failure.
func readForageJSONSandboxName(ctx context.Context, containerName string) string {
	cmd := exec.CommandContext(ctx, "sudo", "machinectl", "shell", containerName, "--", "/bin/cat", "/etc/forage.json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return ""
	}

	var meta forageJSON
	if err := json.Unmarshal(stdout.Bytes(), &meta); err != nil {
		return ""
	}

	return meta.SandboxName
}

// GracefulStop sends SIGTERM via machinectl terminate, waits up to timeout
// for the container to stop, then forces poweroff if still running.
func (r *NspawnRuntime) GracefulStop(ctx context.Context, name string, timeout time.Duration) error {
	containerName := r.containerName(name)
	logging.Debug("graceful stop", "container", containerName, "timeout", timeout)

	// Send SIGTERM to container init via machinectl terminate
	cmd := exec.CommandContext(ctx, "sudo", "machinectl", "terminate", containerName)
	if err := cmd.Run(); err != nil {
		logging.Debug("terminate failed, trying poweroff", "error", err)
		return r.Stop(ctx, name)
	}

	// Poll until stopped or timeout
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		running, err := r.IsRunning(ctx, name)
		if err != nil || !running {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Force stop if still running
	logging.Warn("container did not stop gracefully, forcing poweroff", "container", containerName)
	return r.Stop(ctx, name)
}

// Capabilities returns the full set of capabilities for nspawn.
// NixOS nspawn containers support all features.
func (r *NspawnRuntime) Capabilities() Capabilities {
	return Capabilities{
		NixOSConfig:      true,
		NetworkIsolation: true,
		EphemeralRoot:    true,
		SSHAccess:        true,
		GeneratedFiles:   true,
		ResourceLimits:   true,
		GracefulShutdown: true,
	}
}

// ViewLogs replaces the current process with journalctl -M to view container logs.
func (r *NspawnRuntime) ViewLogs(ctx context.Context, name string, follow bool, lines int) error {
	containerName := r.containerName(name)

	journalctlPath, err := exec.LookPath("journalctl")
	if err != nil {
		return fmt.Errorf("journalctl not found: %w", err)
	}

	argv := []string{"journalctl", "-M", containerName, "-n", fmt.Sprintf("%d", lines)}
	if follow {
		argv = append(argv, "-f")
	}

	return syscall.Exec(journalctlPath, argv, system.SafeEnviron())
}

// Ensure NspawnRuntime implements Runtime, GeneratedFileRuntime, CapableRuntime, GracefulStopper, and LogViewer
var _ Runtime = (*NspawnRuntime)(nil)
var _ GeneratedFileRuntime = (*NspawnRuntime)(nil)
var _ CapableRuntime = (*NspawnRuntime)(nil)
var _ GracefulStopper = (*NspawnRuntime)(nil)
var _ LogViewer = (*NspawnRuntime)(nil)
