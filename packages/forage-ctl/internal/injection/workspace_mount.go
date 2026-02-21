package injection

import (
	"context"
)

// ResolvedMount holds a fully resolved mount spec with effective host paths.
type ResolvedMount struct {
	Name          string
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// WorkspaceMountContributor provides the main workspace mount.
// Deprecated: Use WorkspaceMountsContributor for multi-mount support.
type WorkspaceMountContributor struct {
	WorkspacePath string // Host path to the workspace
	ContainerPath string // Container path (e.g., "/workspace")
}

// NewWorkspaceMountContributor creates a new workspace mount contributor.
// Deprecated: Use NewWorkspaceMountsContributor for multi-mount support.
func NewWorkspaceMountContributor(workspacePath, containerPath string) *WorkspaceMountContributor {
	return &WorkspaceMountContributor{
		WorkspacePath: workspacePath,
		ContainerPath: containerPath,
	}
}

// ContributeMounts returns the workspace mount.
func (w *WorkspaceMountContributor) ContributeMounts(ctx context.Context, req *MountRequest) ([]Mount, error) {
	if w.WorkspacePath == "" {
		return nil, nil
	}
	containerPath := w.ContainerPath
	if containerPath == "" {
		containerPath = "/workspace"
	}
	return []Mount{{
		HostPath:      w.WorkspacePath,
		ContainerPath: containerPath,
		ReadOnly:      req != nil && req.ReadOnlyWorkspace,
	}}, nil
}

// WorkspaceMountsContributor provides multiple workspace mounts.
type WorkspaceMountsContributor struct {
	Mounts []ResolvedMount
}

// NewWorkspaceMountsContributor creates a contributor for multiple workspace mounts.
func NewWorkspaceMountsContributor(mounts []ResolvedMount) *WorkspaceMountsContributor {
	return &WorkspaceMountsContributor{Mounts: mounts}
}

// ContributeMounts returns all workspace mounts.
func (w *WorkspaceMountsContributor) ContributeMounts(ctx context.Context, req *MountRequest) ([]Mount, error) {
	var mounts []Mount
	for _, m := range w.Mounts {
		readOnly := m.ReadOnly
		if req != nil && req.ReadOnlyWorkspace {
			readOnly = true
		}
		mounts = append(mounts, Mount{
			HostPath:      m.HostPath,
			ContainerPath: m.ContainerPath,
			ReadOnly:      readOnly,
		})
	}
	return mounts, nil
}

// Ensure contributors implement MountContributor
var (
	_ MountContributor = (*WorkspaceMountContributor)(nil)
	_ MountContributor = (*WorkspaceMountsContributor)(nil)
)
