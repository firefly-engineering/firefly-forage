package cmd

import (
	"os"
	"path/filepath"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/container"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/workspace"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down <name>",
	Short: "Stop and remove a sandbox",
	Args:  cobra.ExactArgs(1),
	RunE:  runDown,
}

func init() {
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, args []string) error {
	name := args[0]
	paths := config.DefaultPaths()

	logging.Debug("removing sandbox", "name", name)

	hostConfig, err := config.LoadHostConfig(paths.ConfigDir)
	if err != nil {
		return errors.ConfigError("failed to load host config", err)
	}

	metadata, err := config.LoadSandboxMetadata(paths.SandboxesDir, name)
	if err != nil {
		return errors.SandboxNotFound(name)
	}

	// Destroy container if running
	if container.IsRunning(name) {
		logInfo("Stopping container...")
		logging.Debug("destroying container", "name", name)
		if err := container.Destroy(hostConfig.ExtraContainerPath, name); err != nil {
			logging.Warn("failed to destroy container", "error", err)
		}
	}

	// Clean up workspace if using a VCS backend
	if metadata.WorkspaceMode != "" && metadata.WorkspaceMode != "direct" && metadata.SourceRepo != "" {
		var backend workspace.Backend
		switch metadata.WorkspaceMode {
		case "jj":
			backend = workspace.JJ()
		case "git-worktree":
			backend = workspace.Git()
		}

		if backend != nil {
			logInfo("Cleaning up %s workspace...", backend.Name())
			logging.Debug("cleaning up workspace", "backend", backend.Name(), "repo", metadata.SourceRepo, "name", name)
			if err := backend.Remove(metadata.SourceRepo, name, metadata.Workspace); err != nil {
				logging.Warn("failed to cleanup workspace", "error", err)
			}
		}
	}

	// Remove secrets
	secretsPath := filepath.Join(paths.SecretsDir, name)
	logging.Debug("removing secrets", "path", secretsPath)
	os.RemoveAll(secretsPath)

	// Remove skills file
	skillsPath := filepath.Join(paths.SandboxesDir, name+".skills.md")
	os.Remove(skillsPath)

	// Remove nix config file
	configPath := filepath.Join(paths.SandboxesDir, name+".nix")
	os.Remove(configPath)

	// Remove metadata
	logging.Debug("removing metadata")
	if err := config.DeleteSandboxMetadata(paths.SandboxesDir, name); err != nil {
		logging.Warn("failed to remove metadata", "error", err)
	}

	logSuccess("Removed sandbox %s", name)
	return nil
}
