package runtime

import (
	"context"
	"os"
	"path/filepath"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/injection"
)

// SandboxContainerInfo provides runtime-determined paths and settings for containers.
type SandboxContainerInfo struct {
	HomeDir      string // e.g., "/home/agent"
	WorkspaceDir string // e.g., "/workspace"
	Username     string // e.g., "agent"
}

// DefaultContainerInfo returns the default container info for forage sandboxes.
func DefaultContainerInfo() SandboxContainerInfo {
	return SandboxContainerInfo{
		HomeDir:      "/home/agent",
		WorkspaceDir: "/workspace",
		Username:     "agent",
	}
}

// GeneratedFileRuntime extends Runtime with support for generated file mounting.
// Runtimes that support staging generated files for mounting implement this interface.
type GeneratedFileRuntime interface {
	Runtime

	// MountGeneratedFile stages a generated file for mounting into the container.
	// The runtime handles the actual mechanism (e.g., writing to a temp dir that
	// gets bind-mounted, or using container-specific file injection).
	// Returns the mount that will make the file available in the container.
	MountGeneratedFile(ctx context.Context, sandboxName string, file injection.GeneratedFile) (injection.Mount, error)

	// ContainerInfo returns information about the container environment.
	ContainerInfo() SandboxContainerInfo
}

// GeneratedFileMounter provides a default implementation for MountGeneratedFile
// that writes files to a staging directory. Runtimes can embed this to get
// the default behavior.
type GeneratedFileMounter struct {
	// StagingDir is the base directory for staging generated files.
	// Files are written to StagingDir/{sandboxName}/...
	StagingDir string
}

// MountGeneratedFile writes a generated file to the staging directory and
// returns a mount for it.
func (m *GeneratedFileMounter) MountGeneratedFile(ctx context.Context, sandboxName string, file injection.GeneratedFile) (injection.Mount, error) {
	// Create the staging path based on the container path to maintain structure
	// Use the sandbox name to namespace files
	relPath := file.ContainerPath
	if filepath.IsAbs(relPath) {
		relPath = relPath[1:] // Remove leading slash
	}
	hostPath := filepath.Join(m.StagingDir, sandboxName+".generated", relPath)

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(hostPath), 0755); err != nil {
		return injection.Mount{}, err
	}

	// Write the file
	if err := os.WriteFile(hostPath, file.Content, file.Mode); err != nil {
		return injection.Mount{}, err
	}

	return injection.Mount{
		HostPath:      hostPath,
		ContainerPath: file.ContainerPath,
		ReadOnly:      file.ReadOnly,
	}, nil
}
