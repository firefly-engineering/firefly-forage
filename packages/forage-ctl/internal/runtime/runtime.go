// Package runtime defines the container runtime interface for forage-ctl.
// This abstraction allows for multiple backend implementations (nspawn, docker, etc.)
// and enables comprehensive testing through mocking.
package runtime

import (
	"context"
	"io"
	"time"
)

// ContainerStatus represents the state of a container
type ContainerStatus string

const (
	StatusRunning  ContainerStatus = "running"
	StatusStopped  ContainerStatus = "stopped"
	StatusNotFound ContainerStatus = "not-found"
	StatusUnknown  ContainerStatus = "unknown"
)

// ContainerInfo holds information about a container
type ContainerInfo struct {
	Name      string
	Status    ContainerStatus
	StartedAt string
	IPAddress string
}

// ExecResult holds the result of executing a command in a container
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// CreateOptions holds options for creating a container
type CreateOptions struct {
	Name         string
	ConfigPath   string            // Path to container config (e.g., nix file)
	Start        bool              // Start immediately after creation
	BindMounts   map[string]string // host path -> container path
	ForwardPorts map[int]int       // host port -> container port
	NetworkSlot  int               // For private networking
	ExtraArgs    []string          // Backend-specific arguments
}

// ExecOptions holds options for executing a command in a container
type ExecOptions struct {
	User        string    // User to run as
	WorkingDir  string    // Working directory
	Env         []string  // Environment variables
	Stdin       io.Reader // Standard input
	Interactive bool      // Allocate a TTY
}

// Runtime is the interface that container backends must implement.
// All methods should be safe for concurrent use.
type Runtime interface {
	// Name returns the runtime identifier (e.g., "nspawn", "docker")
	Name() string

	// Create creates a new container but does not start it
	Create(ctx context.Context, opts CreateOptions) error

	// Start starts an existing container
	Start(ctx context.Context, name string) error

	// Stop stops a running container
	Stop(ctx context.Context, name string) error

	// Destroy stops and removes a container
	Destroy(ctx context.Context, name string) error

	// IsRunning checks if a container is currently running
	IsRunning(ctx context.Context, name string) (bool, error)

	// Status returns detailed status of a container
	Status(ctx context.Context, name string) (*ContainerInfo, error)

	// Exec executes a command inside a container
	Exec(ctx context.Context, name string, command []string, opts ExecOptions) (*ExecResult, error)

	// ExecInteractive executes a command with an interactive TTY
	// This replaces the current process (uses syscall.Exec)
	ExecInteractive(ctx context.Context, name string, command []string, opts ExecOptions) error

	// List returns all containers managed by this runtime
	List(ctx context.Context) ([]*ContainerInfo, error)
}

// Capabilities describes what features a runtime supports.
// Runtimes return this from the Capabilities() method so callers can
// gate features or warn when a template requests unsupported functionality.
type Capabilities struct {
	NixOSConfig      bool // Can generate NixOS container configs
	NetworkIsolation bool // Supports network mode filtering
	EphemeralRoot    bool // Root filesystem is ephemeral
	SSHAccess        bool // Supports SSH into container
	GeneratedFiles   bool // Supports generated file mounting
	ResourceLimits   bool // Supports cgroup resource limits
	GracefulShutdown bool // Supports graceful stop signals
}

// CapableRuntime is an optional interface that runtimes can implement to
// advertise their capabilities. Runtimes that do not implement this are
// assumed to have full capabilities.
type CapableRuntime interface {
	Capabilities() Capabilities
}

// GetCapabilities returns the capabilities of a runtime.
// If the runtime implements CapableRuntime, its declared capabilities are returned.
// Otherwise, all capabilities are assumed to be true (backward compatibility).
func GetCapabilities(rt Runtime) Capabilities {
	if cr, ok := rt.(CapableRuntime); ok {
		return cr.Capabilities()
	}
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

// GracefulStopper is an optional interface for runtimes that support
// graceful shutdown with a configurable timeout. If not implemented,
// callers should fall back to Stop() for immediate termination.
type GracefulStopper interface {
	GracefulStop(ctx context.Context, name string, timeout time.Duration) error
}

// SSHRuntime extends Runtime with SSH-based access capabilities.
// This is used by runtimes that provide SSH access to containers.
type SSHRuntime interface {
	Runtime

	// SSHHost returns the SSH host (container IP) for a container
	SSHHost(ctx context.Context, name string) (string, error)

	// SSHExec executes a command via SSH
	SSHExec(ctx context.Context, name string, command []string, opts ExecOptions) (*ExecResult, error)

	// SSHInteractive starts an interactive SSH session
	SSHInteractive(ctx context.Context, name string, command string) error
}
