package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/app"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
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

	metadata, err := loadSandbox(name)
	if err != nil {
		return err
	}

	// Stop the container if running
	if isRunning(cmd.Context(), name) {
		logInfo("Stopping container...")
		logging.Debug("destroying container", "name", name)
		if err := app.Default.Destroy(cmd.Context(), name); err != nil {
			logWarning("Failed to stop container: %v", err)
		}
	}

	// Restart the container (uses cached etc → outer config → full .nix fallback)
	logInfo("Starting container...")
	if err := app.Default.Start(cmd.Context(), name); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for SSH to be ready
	logInfo("Waiting for sandbox to be ready...")
	ready := false
	for i := 0; i < health.SSHReadyTimeoutSeconds; i++ {
		if health.CheckSSH(metadata.ContainerIP()) {
			ready = true
			break
		}
		time.Sleep(time.Second)
	}

	if !ready {
		logWarning("SSH not ready after %d seconds", health.SSHReadyTimeoutSeconds)
	}

	logSuccess("Reset sandbox %s", name)
	return nil
}
