// Package sandbox provides high-level sandbox lifecycle management.
package sandbox

import (
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/workspace"
)

// CreateOptions holds all options for creating a sandbox.
type CreateOptions struct {
	// Name is the sandbox name (required)
	Name string

	// Template is the template name to use (required)
	Template string

	// WorkspaceMode specifies how the workspace is set up
	WorkspaceMode WorkspaceMode

	// WorkspacePath is the path for direct workspace mode
	WorkspacePath string

	// RepoPath is the repository path for jj/git-worktree modes
	RepoPath string
}

// WorkspaceMode specifies the workspace setup strategy.
type WorkspaceMode string

const (
	// WorkspaceModeDirect mounts a directory directly
	WorkspaceModeDirect WorkspaceMode = "direct"

	// WorkspaceModeJJ creates a jj workspace
	WorkspaceModeJJ WorkspaceMode = "jj"

	// WorkspaceModeGitWorktree creates a git worktree
	WorkspaceModeGitWorktree WorkspaceMode = "git-worktree"
)

// CreateResult holds the result of a successful sandbox creation.
type CreateResult struct {
	// Name is the sandbox name
	Name string

	// Port is the allocated SSH port
	Port int

	// Workspace is the effective workspace path
	Workspace string

	// Metadata is the full sandbox metadata
	Metadata *config.SandboxMetadata
}

// workspaceBackendFor returns the appropriate workspace backend for a mode.
func workspaceBackendFor(mode WorkspaceMode) workspace.Backend {
	switch mode {
	case WorkspaceModeJJ:
		return workspace.JJ()
	case WorkspaceModeGitWorktree:
		return workspace.Git()
	default:
		return nil
	}
}
