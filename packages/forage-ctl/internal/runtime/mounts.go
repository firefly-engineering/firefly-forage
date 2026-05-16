// Package runtime provides container runtime implementations.
// This file defines the unified bind mount interface.
package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
)

// MountType specifies how a path is mounted into the container
type MountType string

const (
	// MountBind creates a bind mount from host to container
	MountBind MountType = "bind"
	// MountVolume uses a named volume (Docker/Podman only)
	MountVolume MountType = "volume"
	// MountTmpfs creates a tmpfs mount
	MountTmpfs MountType = "tmpfs"
)

// Mount represents a filesystem mount in a container
type Mount struct {
	// Type is the mount type (bind, volume, tmpfs)
	Type MountType

	// Source is the host path (for bind) or volume name (for volume)
	Source string

	// Target is the path inside the container
	Target string

	// ReadOnly makes the mount read-only
	ReadOnly bool

	// Options are additional mount options (runtime-specific)
	Options map[string]string
}

// StandardMounts returns the standard mounts required for a sandbox.
// This includes nix store, workspace, secrets, etc.
type StandardMounts struct {
	// NixStore is the nix store mount
	NixStore *Mount

	// NixDaemonSocket is the nix daemon socket for builds
	NixDaemonSocket *Mount

	// Workspace is the project workspace mount
	Workspace *Mount

	// Secrets is the secrets directory mount
	Secrets *Mount

	// SourceRepo is for jj/git workspace mode (.jj or .git directory)
	SourceRepo *Mount
}

// NewStandardMounts creates the standard mounts for a sandbox
func NewStandardMounts(workspace, secretsPath, sourceRepo string) *StandardMounts {
	mounts := &StandardMounts{
		NixStore: &Mount{
			Type:     MountBind,
			Source:   "/nix/store",
			Target:   "/nix/store",
			ReadOnly: true,
		},
		NixDaemonSocket: &Mount{
			Type:   MountBind,
			Source: "/nix/var/nix/daemon-socket",
			Target: "/nix/var/nix/daemon-socket",
		},
		Workspace: &Mount{
			Type:   MountBind,
			Source: workspace,
			Target: "/workspace",
		},
		Secrets: &Mount{
			Type:     MountBind,
			Source:   secretsPath,
			Target:   "/run/secrets",
			ReadOnly: true,
		},
	}

	// For jj/git workspace mode, mount the source repo's VCS directory
	if sourceRepo != "" {
		jjPath := filepath.Join(sourceRepo, ".jj")
		if _, err := os.Stat(jjPath); err == nil {
			mounts.SourceRepo = &Mount{
				Type:   MountBind,
				Source: jjPath,
				Target: jjPath, // Same path to preserve symlinks
			}
		} else {
			// Check for .git
			gitPath := filepath.Join(sourceRepo, ".git")
			if _, err := os.Stat(gitPath); err == nil {
				mounts.SourceRepo = &Mount{
					Type:   MountBind,
					Source: gitPath,
					Target: gitPath,
				}
			}
		}
	}

	return mounts
}

// ToNspawnConfig converts mounts to NixOS container config format
func (m *StandardMounts) ToNspawnConfig() map[string]interface{} {
	config := make(map[string]interface{})

	addMount := func(mount *Mount) {
		if mount == nil {
			return
		}
		config[mount.Target] = map[string]interface{}{
			"hostPath":   mount.Source,
			"isReadOnly": mount.ReadOnly,
		}
	}

	addMount(m.NixStore)
	addMount(m.NixDaemonSocket)
	addMount(m.Workspace)
	addMount(m.Secrets)
	addMount(m.SourceRepo)

	return config
}

// ToDockerArgs converts mounts to Docker/Podman command line arguments
func (m *StandardMounts) ToDockerArgs() []string {
	var args []string

	addMount := func(mount *Mount) {
		if mount == nil {
			return
		}
		mountStr := fmt.Sprintf("%s:%s", mount.Source, mount.Target)
		if mount.ReadOnly {
			mountStr += ":ro"
		}
		args = append(args, "-v", mountStr)
	}

	addMount(m.NixStore)
	addMount(m.NixDaemonSocket)
	addMount(m.Workspace)
	addMount(m.Secrets)
	addMount(m.SourceRepo)

	return args
}

// ToAppleArgs converts mounts to Apple Container command line arguments
func (m *StandardMounts) ToAppleArgs() []string {
	var args []string

	addMount := func(mount *Mount) {
		if mount == nil {
			return
		}
		mountStr := fmt.Sprintf("type=bind,source=%s,target=%s", mount.Source, mount.Target)
		if mount.ReadOnly {
			mountStr += ",readonly"
		}
		args = append(args, "--mount", mountStr)
	}

	addMount(m.NixStore)
	addMount(m.NixDaemonSocket)
	addMount(m.Workspace)
	addMount(m.Secrets)
	addMount(m.SourceRepo)

	return args
}

// NixStoreStrategy describes how the nix store is shared with containers
type NixStoreStrategy string

const (
	// NixStoreBindMount uses direct bind mount (NixOS, Linux with nix installed)
	NixStoreBindMount NixStoreStrategy = "bind"

	// NixStoreVolume uses a named Docker volume (for environments without nix)
	NixStoreVolume NixStoreStrategy = "volume"

	// NixStoreDeterminate uses Determinate Nix installer paths (macOS recommended)
	NixStoreDeterminate NixStoreStrategy = "determinate"
)

// DetectNixStoreStrategy determines the best strategy for sharing the nix store
func DetectNixStoreStrategy() NixStoreStrategy {
	// Check if nix store exists at standard path
	if _, err := os.Stat("/nix/store"); err == nil {
		// Check for Determinate Nix (macOS)
		if goruntime.GOOS == "darwin" {
			// Determinate Nix uses a volume mounted at /nix
			if _, err := os.Stat("/nix/.nix-netrc-file"); err == nil {
				return NixStoreDeterminate
			}
		}
		return NixStoreBindMount
	}

	// Fallback to volume (nix must be installed in container)
	return NixStoreVolume
}

// PlatformMountConfig holds platform-specific mount configuration
type PlatformMountConfig struct {
	// NixStorePath is the nix store path (usually /nix/store)
	NixStorePath string

	// NixDaemonSocketPath is the daemon socket path
	NixDaemonSocketPath string

	// Strategy is how the nix store is shared
	Strategy NixStoreStrategy

	// UseBindMount indicates if bind mounts work on this platform
	UseBindMount bool
}

// DetectPlatformMountConfig returns the mount configuration for the current platform
func DetectPlatformMountConfig() *PlatformMountConfig {
	config := &PlatformMountConfig{
		NixStorePath:        "/nix/store",
		NixDaemonSocketPath: "/nix/var/nix/daemon-socket",
		Strategy:            DetectNixStoreStrategy(),
		UseBindMount:        true,
	}

	switch goruntime.GOOS {
	case "darwin":
		// macOS may need special handling depending on nix installer
		if config.Strategy == NixStoreDeterminate {
			// Determinate Nix handles this correctly
			config.UseBindMount = true
		}
	case "linux":
		// Linux with nix works with bind mounts
		config.UseBindMount = true
	}

	return config
}
