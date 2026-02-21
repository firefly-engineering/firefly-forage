// Package sandbox provides sandbox lifecycle management for forage-ctl.
//
// This package handles the creation and configuration of isolated sandbox
// environments, including workspace setup, secret injection, container
// creation, and health monitoring.
//
// # Creator
//
// Creator orchestrates sandbox creation with all necessary dependencies:
//
//	creator, err := sandbox.NewCreator()
//	if err != nil {
//	    return err
//	}
//
//	result, err := creator.Create(ctx, sandbox.CreateOptions{
//	    Name:          "my-sandbox",
//	    Template:      "claude",
//	    WorkspaceMode: sandbox.WorkspaceModeDirect,
//	    WorkspacePath: "/path/to/workspace",
//	})
//
// # Workspace Modes
//
// Three workspace modes are supported:
//
//   - WorkspaceModeDirect: Bind-mount an existing directory directly
//   - WorkspaceModeJJ: Create an isolated jj workspace from a repo
//   - WorkspaceModeGitWorktree: Create an isolated git worktree from a repo
//
// # Creation Flow
//
// The Creator.Create method:
//  1. Validates inputs and loads template
//  2. Allocates port and network slot
//  3. Sets up workspace (creates jj workspace or git worktree if needed)
//  4. Copies secrets to sandbox secrets directory
//  5. Generates Nix container configuration
//  6. Creates and starts container via runtime
//  7. Saves sandbox metadata
//  8. Waits for SSH to become ready
//  9. Injects project-aware skills file
//
// On failure, cleanup is automatic: secrets, configs, and partial containers
// are removed.
package sandbox
