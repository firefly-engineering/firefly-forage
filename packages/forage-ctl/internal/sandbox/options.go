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

	// RepoPath is the target path (repo or directory)
	RepoPath string

	// Direct forces direct mount, skipping VCS isolation
	Direct bool

	// SSHKeys are explicit SSH public keys for sandbox access (optional)
	// If empty, keys are resolved from config or ~/.ssh/*.pub
	SSHKeys []string

	// NoMuxConfig skips mounting the host multiplexer config into the sandbox
	NoMuxConfig bool

	// GitUser is the git user.name for agent commits (optional)
	GitUser string

	// GitEmail is the git user.email for agent commits (optional)
	GitEmail string

	// SSHKeyPath is the absolute path to a private SSH key on the host (optional)
	SSHKeyPath string
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

	// ContainerIP is the container's IP address for SSH access
	ContainerIP string

	// Workspace is the effective workspace path
	Workspace string

	// Metadata is the full sandbox metadata
	Metadata *config.SandboxMetadata

	// CapabilityWarnings lists features the runtime doesn't support
	CapabilityWarnings []string
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
