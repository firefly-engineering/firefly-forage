package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl-go/internal/container"
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

	hostConfig, err := config.LoadHostConfig(paths.ConfigDir)
	if err != nil {
		return fmt.Errorf("failed to load host config: %w", err)
	}

	metadata, err := config.LoadSandboxMetadata(paths.SandboxesDir, name)
	if err != nil {
		return fmt.Errorf("sandbox not found: %s", name)
	}

	// Destroy container if running
	if container.IsRunning(name) {
		logInfo("Stopping container...")
		if err := container.Destroy(hostConfig.ExtraContainerPath, name); err != nil {
			logWarning("Failed to destroy container: %v", err)
		}
	}

	// Clean up jj workspace if applicable
	if metadata.WorkspaceMode == "jj" && metadata.SourceRepo != "" {
		logInfo("Cleaning up jj workspace...")
		if err := cleanupJJWorkspace(metadata.SourceRepo, metadata.JJWorkspaceName); err != nil {
			logWarning("Failed to cleanup jj workspace: %v", err)
		}

		// Remove workspace directory
		if metadata.Workspace != "" {
			os.RemoveAll(metadata.Workspace)
		}
	}

	// Remove secrets
	secretsPath := filepath.Join(paths.SecretsDir, name)
	os.RemoveAll(secretsPath)

	// Remove skills file
	skillsPath := filepath.Join(paths.SandboxesDir, name+".skills.md")
	os.Remove(skillsPath)

	// Remove metadata
	if err := config.DeleteSandboxMetadata(paths.SandboxesDir, name); err != nil {
		logWarning("Failed to remove metadata: %v", err)
	}

	logSuccess("Removed sandbox %s", name)
	return nil
}

func cleanupJJWorkspace(repoPath, workspaceName string) error {
	if workspaceName == "" {
		return nil
	}

	cmd := exec.Command("jj", "workspace", "forget", workspaceName, "-R", repoPath)
	return cmd.Run()
}
