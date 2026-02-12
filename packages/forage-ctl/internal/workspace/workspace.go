// Package workspace provides a common interface for VCS workspace backends
package workspace

import (
	"fmt"
	"regexp"
)

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

// DetectBackend returns the appropriate workspace backend for the given path,
// or nil if no backend recognizes it as a repository.
// Checks jj first (since jj repos also contain .git).
func DetectBackend(path string) Backend {
	jj := &JJBackend{}
	if jj.IsRepo(path) {
		return jj
	}
	git := &GitBackend{}
	if git.IsRepo(path) {
		return git
	}
	return nil
}

// validName matches safe workspace/branch names: alphanumeric, hyphens, underscores, dots.
var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// ValidateName checks that a workspace name is safe for use in branch names,
// directory paths, and shell commands. This is a defense-in-depth check at the
// backend interface boundary.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("workspace name must not be empty")
	}
	if len(name) > 128 {
		return fmt.Errorf("workspace name too long (max 128 characters)")
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("workspace name %q contains invalid characters (allowed: alphanumeric, hyphens, underscores, dots)", name)
	}
	return nil
}

// WorkspaceInfo contains information about a created workspace
type WorkspaceInfo struct {
	// Path is the filesystem path to the workspace
	Path string

	// Branch is the git branch name (git-worktree only)
	Branch string
}
