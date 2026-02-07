package sandbox

import (
	"context"
	"os"
	"path/filepath"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/workspace"
)

// CleanupOptions configures sandbox cleanup behavior.
type CleanupOptions struct {
	// DestroyContainer if true, destroys the container via runtime.
	DestroyContainer bool

	// CleanupWorkspace if true, removes VCS workspace (jj/git-worktree).
	CleanupWorkspace bool

	// CleanupSecrets if true, removes the secrets directory.
	CleanupSecrets bool

	// CleanupConfig if true, removes the nix config file.
	CleanupConfig bool

	// CleanupSkills if true, removes the skills markdown file.
	CleanupSkills bool

	// CleanupPermissions if true, removes agent permissions files (*-permissions.json).
	CleanupPermissions bool

	// CleanupMetadata if true, removes the sandbox metadata file.
	CleanupMetadata bool
}

// DefaultCleanupOptions returns options that clean up everything.
func DefaultCleanupOptions() CleanupOptions {
	return CleanupOptions{
		DestroyContainer:   true,
		CleanupWorkspace:   true,
		CleanupSecrets:     true,
		CleanupConfig:      true,
		CleanupSkills:      true,
		CleanupPermissions: true,
		CleanupMetadata:    true,
	}
}

// Cleanup removes sandbox resources.
// This is the canonical cleanup function used by both the down command
// and error recovery in the create flow.
// The rt parameter is optional; if nil, container destruction is skipped.
func Cleanup(metadata *config.SandboxMetadata, paths *config.Paths, opts CleanupOptions, rt runtime.Runtime) {
	if metadata == nil {
		return
	}

	name := metadata.Name
	logging.Debug("cleaning up sandbox", "name", name)

	// Destroy container if requested and running
	if opts.DestroyContainer && rt != nil {
		running, _ := rt.IsRunning(context.Background(), name)
		if running {
			logging.Debug("destroying container", "name", name)
			if err := rt.Destroy(context.Background(), name); err != nil {
				logging.Debug("container destroy during cleanup", "error", err)
			}
		}
	}

	// Clean up workspace if using a VCS backend
	if opts.CleanupWorkspace && metadata.SourceRepo != "" {
		backend := workspaceBackendForMode(metadata.WorkspaceMode)
		if backend != nil {
			logging.Debug("cleaning up workspace",
				"backend", backend.Name(),
				"repo", metadata.SourceRepo,
				"name", name)
			if err := backend.Remove(metadata.SourceRepo, name, metadata.Workspace); err != nil {
				logging.Warn("failed to remove workspace", "error", err)
			}
		}
	}

	// Remove secrets directory
	if opts.CleanupSecrets {
		secretsPath := filepath.Join(paths.SecretsDir, name)
		logging.Debug("removing secrets", "path", secretsPath)
		if err := os.RemoveAll(secretsPath); err != nil {
			logging.Warn("failed to remove secrets directory", "path", secretsPath, "error", err)
		}
	}

	// Remove skills file
	if opts.CleanupSkills {
		skillsPath := filepath.Join(paths.SandboxesDir, name+".skills.md")
		if err := os.Remove(skillsPath); err != nil && !os.IsNotExist(err) {
			logging.Warn("failed to remove skills file", "path", skillsPath, "error", err)
		}
	}

	// Remove permissions files (e.g. <name>.claude-permissions.json)
	if opts.CleanupPermissions {
		pattern := filepath.Join(paths.SandboxesDir, name+".*-permissions.json")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			logging.Warn("failed to glob permissions files", "pattern", pattern, "error", err)
		}
		for _, match := range matches {
			logging.Debug("removing permissions file", "path", match)
			if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
				logging.Warn("failed to remove permissions file", "path", match, "error", err)
			}
		}
	}

	// Remove nix config file
	if opts.CleanupConfig {
		configPath := filepath.Join(paths.SandboxesDir, name+".nix")
		if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
			logging.Warn("failed to remove config file", "path", configPath, "error", err)
		}
	}

	// Remove metadata
	if opts.CleanupMetadata {
		logging.Debug("removing metadata", "name", name)
		if err := config.DeleteSandboxMetadata(paths.SandboxesDir, name); err != nil {
			logging.Warn("failed to remove metadata", "name", name, "error", err)
		}
	}
}

// workspaceBackendForMode returns the workspace backend for a given mode string.
func workspaceBackendForMode(mode string) workspace.Backend {
	switch mode {
	case "jj":
		return workspace.JJ()
	case "git-worktree":
		return workspace.Git()
	default:
		return nil
	}
}
