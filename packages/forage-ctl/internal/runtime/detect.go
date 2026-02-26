package runtime

import (
	"fmt"
	"os"
	"os/exec"
	goruntime "runtime"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
)

// RuntimeType identifies which container runtime to use
type RuntimeType string

const (
	RuntimeNspawn RuntimeType = "nspawn"
	RuntimeDocker RuntimeType = "docker"
	RuntimePodman RuntimeType = "podman"
	RuntimeApple  RuntimeType = "apple"
	RuntimeAuto   RuntimeType = "auto"
)

// Config holds runtime configuration
type Config struct {
	// Type specifies which runtime to use (or "auto" for auto-detection)
	Type RuntimeType

	// ContainerPrefix is prepended to sandbox names
	ContainerPrefix string

	// NixpkgsPath is the Nix store path to nixpkgs source (nspawn only)
	// Used for nix-build of container configurations
	NixpkgsPath string

	// SandboxesDir is the directory containing sandbox metadata files
	// Used by all runtimes to resolve container names from metadata
	SandboxesDir string
}

// DefaultConfig returns the default runtime configuration
func DefaultConfig() *Config {
	return &Config{
		Type:            RuntimeAuto,
		ContainerPrefix: "forage-",
	}
}

// Detect determines which container runtime is available on the system.
// Returns the RuntimeType and any error encountered.
func Detect() (RuntimeType, error) {
	logging.Debug("detecting container runtime", "os", goruntime.GOOS)

	switch goruntime.GOOS {
	case "linux":
		return detectLinux()
	case "darwin":
		return detectDarwin()
	default:
		return "", fmt.Errorf("unsupported operating system: %s", goruntime.GOOS)
	}
}

// detectLinux detects the best runtime for Linux systems
func detectLinux() (RuntimeType, error) {
	// On NixOS with systemd, prefer nspawn
	if isNixOS() && hasSystemd() {
		logging.Debug("detected NixOS with systemd, using nspawn")
		return RuntimeNspawn, nil
	}

	// Try podman (preferred for rootless)
	if _, err := exec.LookPath("podman"); err == nil {
		logging.Debug("detected podman")
		return RuntimePodman, nil
	}

	// Try docker
	if _, err := exec.LookPath("docker"); err == nil {
		logging.Debug("detected docker")
		return RuntimeDocker, nil
	}

	return "", fmt.Errorf("no supported container runtime found (tried: nspawn, podman, docker)")
}

// detectDarwin detects the best runtime for macOS
func detectDarwin() (RuntimeType, error) {
	// Prefer Apple Container if available (native macOS virtualization)
	if _, err := exec.LookPath("container"); err == nil {
		logging.Debug("detected Apple Container on macOS")
		return RuntimeApple, nil
	}

	// Try podman
	if _, err := exec.LookPath("podman"); err == nil {
		logging.Debug("detected podman on macOS")
		return RuntimePodman, nil
	}

	// Try docker (Docker Desktop)
	if _, err := exec.LookPath("docker"); err == nil {
		logging.Debug("detected docker on macOS")
		return RuntimeDocker, nil
	}

	return "", fmt.Errorf("no supported container runtime found on macOS (tried: container, podman, docker)")
}

// isNixOS checks if we're running on NixOS
func isNixOS() bool {
	// Check for /etc/NIXOS marker file
	if _, err := os.Stat("/etc/NIXOS"); err == nil {
		return true
	}

	// Check for /run/current-system (NixOS-specific)
	if _, err := os.Stat("/run/current-system"); err == nil {
		return true
	}

	return false
}

// New creates a new Runtime based on the configuration.
// If Type is RuntimeAuto, it auto-detects the best runtime.
func New(cfg *Config) (Runtime, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	runtimeType := cfg.Type
	if runtimeType == RuntimeAuto {
		detected, err := Detect()
		if err != nil {
			return nil, err
		}
		runtimeType = detected
	}

	logging.Debug("creating runtime", "type", runtimeType)

	switch runtimeType {
	case RuntimeNspawn:
		return NewNspawnRuntime(cfg.ContainerPrefix, cfg.SandboxesDir, cfg.NixpkgsPath), nil

	case RuntimeDocker, RuntimePodman:
		return NewDockerRuntime(cfg.ContainerPrefix, cfg.SandboxesDir)

	case RuntimeApple:
		return NewAppleRuntime(cfg.ContainerPrefix, cfg.SandboxesDir)

	default:
		return nil, fmt.Errorf("unknown runtime type: %s", runtimeType)
	}
}

// MustNew creates a new Runtime, panicking on error.
// Useful for initialization in main or tests.
func MustNew(cfg *Config) Runtime {
	rt, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return rt
}

// hasSystemd checks if systemd is running.
// https://www.freedesktop.org/software/systemd/man/sd_booted.html
func hasSystemd() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

// Available returns a list of available runtimes on this system
func Available() []RuntimeType {
	var available []RuntimeType

	if goruntime.GOOS == "linux" && isNixOS() && hasSystemd() {
		available = append(available, RuntimeNspawn)
	}

	// Check for Apple Container on macOS
	if goruntime.GOOS == "darwin" {
		if _, err := exec.LookPath("container"); err == nil {
			available = append(available, RuntimeApple)
		}
	}

	if _, err := exec.LookPath("podman"); err == nil {
		available = append(available, RuntimePodman)
	}

	if _, err := exec.LookPath("docker"); err == nil {
		available = append(available, RuntimeDocker)
	}

	return available
}
