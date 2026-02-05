// Package workspace provides a common interface for VCS workspace backends
package workspace

// Backend provides isolated working directories for a version control system
type Backend interface {
	// Name returns the backend name (e.g., "jj", "git-worktree")
	Name() string

	// IsRepo checks if path is a valid repository for this backend
	IsRepo(path string) bool

	// Exists checks if a workspace with this name already exists
	Exists(repoPath, name string) bool

	// Create creates an isolated workspace at workspacePath
	// For git, this creates a branch named after the workspace
	// For jj, this creates a named workspace
	Create(repoPath, name, workspacePath string) error

	// Remove cleans up the workspace and any associated resources
	// For git, this removes the worktree and deletes the branch
	// For jj, this forgets the workspace
	Remove(repoPath, name, workspacePath string) error
}

// WorkspaceInfo contains information about a created workspace
type WorkspaceInfo struct {
	// Path is the filesystem path to the workspace
	Path string

	// Branch is the git branch name (git-worktree only)
	Branch string
}
