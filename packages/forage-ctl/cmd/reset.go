package cmd

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/spf13/cobra"
)

var resetCmd = &cobra.Command{
	Use:   "reset <name>",
	Short: "Reset sandbox (restart with fresh ephemeral state)",
	Args:  cobra.ExactArgs(1),
	RunE:  runReset,
}

func init() {
	rootCmd.AddCommand(resetCmd)
}

func runReset(cmd *cobra.Command, args []string) error {
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

	// Stop the container if running
	if runtime.IsRunning(name) {
		logInfo("Stopping container...")
		logging.Debug("destroying container", "name", name)
		if err := runtime.Destroy(name); err != nil {
			logWarning("Failed to stop container: %v", err)
		}
	}

	// Restart the container
	logInfo("Starting container...")
	containerName := config.ContainerName(name)

	// The container config should still exist in the sandboxes directory
	configPath := fmt.Sprintf("%s/%s.nix", paths.SandboxesDir, name)
	createCmd := exec.Command("sudo", hostConfig.ExtraContainerPath, "create", "--start", configPath)
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for SSH to be ready
	logInfo("Waiting for sandbox to be ready...")
	ready := false
	for i := 0; i < 30; i++ {
		if health.CheckSSH(metadata.Port) {
			ready = true
			break
		}
		time.Sleep(time.Second)
	}

	if !ready {
		logWarning("SSH not ready after 30 seconds")
	}

	logSuccess("Reset sandbox %s (%s)", name, containerName)
	return nil
}
