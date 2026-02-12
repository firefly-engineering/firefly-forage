package injection

import (
	"context"
)

// WorkspaceMountContributor provides the main workspace mount.
type WorkspaceMountContributor struct {
	WorkspacePath string // Host path to the workspace
	ContainerPath string // Container path (e.g., "/workspace")
}

// NewWorkspaceMountContributor creates a new workspace mount contributor.
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

// Ensure WorkspaceMountContributor implements MountContributor
var _ MountContributor = (*WorkspaceMountContributor)(nil)
