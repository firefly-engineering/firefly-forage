// Package workspace provides a common interface for VCS workspace backends.
//
// This package abstracts the creation and management of isolated working
// directories backed by different version control systems.
//
// # Backend Interface
//
// The Backend interface defines operations for workspace management:
//
//	type Backend interface {
//	    Name() string                                    // "jj" or "git-worktree"
//	    IsRepo(path string) bool                         // Check for valid repo
//	    Exists(repoPath, name string) bool               // Check workspace exists
//	    Create(repoPath, name, workspacePath string) error
//	    Remove(repoPath, name, workspacePath string) error
//	}
//
// # JJ Backend
//
// JJBackend creates isolated jj workspaces:
//
//	backend := &workspace.JJBackend{}
//	backend.Create("/path/to/repo", "sandbox-1", "/var/lib/forage/workspaces/sandbox-1")
//	// Creates: jj workspace add --name sandbox-1 /var/lib/forage/workspaces/sandbox-1
//
// # Git Backend
//
// GitBackend creates isolated git worktrees with dedicated branches:
//
//	backend := &workspace.GitBackend{}
//	backend.Create("/path/to/repo", "sandbox-1", "/var/lib/forage/workspaces/sandbox-1")
//	// Creates: git worktree add /var/lib/forage/workspaces/sandbox-1 -b forage/sandbox-1
//
// # Workspace Modes
//
// Sandboxes use one of three workspace modes:
//   - direct: Bind-mount an existing directory (no backend)
//   - jj: Create an isolated jj workspace (JJBackend)
//   - git-worktree: Create an isolated git worktree (GitBackend)
package workspace
