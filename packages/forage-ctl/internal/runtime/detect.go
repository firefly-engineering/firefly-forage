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
	RuntimeAuto   RuntimeType = "auto"
)

// Config holds runtime configuration
type Config struct {
	// Type specifies which runtime to use (or "auto" for auto-detection)
	Type RuntimeType

	// ContainerPrefix is prepended to sandbox names
	ContainerPrefix string

	// ExtraContainerPath is the path to extra-container binary (nspawn only)
	ExtraContainerPath string
}

// DefaultConfig returns the default runtime configuration
func DefaultConfig() *Config {
	return &Config{
		Type:               RuntimeAuto,
		ContainerPrefix:    "forage-",
		ExtraContainerPath: "/run/current-system/sw/bin/extra-container",
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
	// On NixOS, prefer nspawn if extra-container is available
	if isNixOS() {
		if _, err := exec.LookPath("extra-container"); err == nil {
			logging.Debug("detected NixOS with extra-container")
			return RuntimeNspawn, nil
		}
		// Check common NixOS paths
		paths := []string{
			"/run/current-system/sw/bin/extra-container",
			"/etc/profiles/per-user/root/bin/extra-container",
		}
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				logging.Debug("detected NixOS with extra-container", "path", path)
				return RuntimeNspawn, nil
			}
		}
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

	return "", fmt.Errorf("no supported container runtime found (tried: extra-container, podman, docker)")
}

// detectDarwin detects the best runtime for macOS
func detectDarwin() (RuntimeType, error) {
	// TODO: Add Apple Container support when available
	// For now, fall back to Docker Desktop or Podman

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

	return "", fmt.Errorf("no supported container runtime found on macOS (tried: podman, docker)")
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
		path := cfg.ExtraContainerPath
		if path == "" {
			// Try to find extra-container
			if p, err := exec.LookPath("extra-container"); err == nil {
				path = p
			} else {
				path = "/run/current-system/sw/bin/extra-container"
			}
		}
		return NewNspawnRuntime(path, cfg.ContainerPrefix), nil

	case RuntimeDocker, RuntimePodman:
		return NewDockerRuntime(cfg.ContainerPrefix)

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

// Available returns a list of available runtimes on this system
func Available() []RuntimeType {
	var available []RuntimeType

	if goruntime.GOOS == "linux" && isNixOS() {
		paths := []string{
			"/run/current-system/sw/bin/extra-container",
			"/etc/profiles/per-user/root/bin/extra-container",
		}
		for _, path := range paths {
			if _, err := os.Stat(path); err == nil {
				available = append(available, RuntimeNspawn)
				break
			}
		}
		if _, err := exec.LookPath("extra-container"); err == nil {
			// Check if already added
			found := false
			for _, rt := range available {
				if rt == RuntimeNspawn {
					found = true
					break
				}
			}
			if !found {
				available = append(available, RuntimeNspawn)
			}
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
