package runtime

import (
	"context"
	"fmt"
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
// returns a mount for it. It validates that no symlinks exist in the path
// to prevent symlink-based attacks that could redirect writes to arbitrary
// host locations.
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

	// Validate no symlinks in the resolved path to prevent TOCTOU attacks.
	// EvalSymlinks resolves all symlinks and gives us the real path.
	realDir, err := filepath.EvalSymlinks(filepath.Dir(hostPath))
	if err != nil {
		return injection.Mount{}, fmt.Errorf("failed to resolve staging path: %w", err)
	}
	expectedDir, err := filepath.EvalSymlinks(m.StagingDir)
	if err != nil {
		return injection.Mount{}, fmt.Errorf("failed to resolve staging base: %w", err)
	}
	if !isSubpath(realDir, expectedDir) {
		return injection.Mount{}, fmt.Errorf("staging path escapes base directory: %s", realDir)
	}

	// Write with O_CREATE|O_EXCL first (fails if file exists, including symlinks).
	// Fall back to O_TRUNC if file already exists from a previous run.
	realHostPath := filepath.Join(realDir, filepath.Base(hostPath))
	f, err := os.OpenFile(realHostPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, file.Mode)
	if os.IsExist(err) {
		// File exists from previous run; verify it's not a symlink before overwriting
		info, statErr := os.Lstat(realHostPath)
		if statErr != nil {
			return injection.Mount{}, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return injection.Mount{}, fmt.Errorf("refusing to overwrite symlink: %s", realHostPath)
		}
		f, err = os.OpenFile(realHostPath, os.O_WRONLY|os.O_TRUNC, file.Mode)
	}
	if err != nil {
		return injection.Mount{}, err
	}
	_, writeErr := f.Write(file.Content)
	closeErr := f.Close()
	if writeErr != nil {
		return injection.Mount{}, writeErr
	}
	if closeErr != nil {
		return injection.Mount{}, closeErr
	}

	return injection.Mount{
		HostPath:      realHostPath,
		ContainerPath: file.ContainerPath,
		ReadOnly:      file.ReadOnly,
	}, nil
}

// isSubpath returns true if child is under parent (or equal to parent).
func isSubpath(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (len(rel) > 0 && rel[0] != '.' && !filepath.IsAbs(rel))
}
