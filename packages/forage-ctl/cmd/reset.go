package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/health"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
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

	// The container config should still exist in the sandboxes directory
	configPath := fmt.Sprintf("%s/%s.nix", paths.SandboxesDir, name)
	logging.Debug("creating container via runtime", "name", name, "config", configPath)
	if err := runtime.Create(runtime.CreateOptions{
		Name:       name,
		ConfigPath: configPath,
		Start:      true,
	}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Wait for SSH to be ready
	logInfo("Waiting for sandbox to be ready...")
	ready := false
	for i := 0; i < health.SSHReadyTimeoutSeconds; i++ {
		if health.CheckSSH(metadata.Port) {
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
