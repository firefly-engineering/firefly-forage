package cmd

import (
	"fmt"
	"os/exec"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/container"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a stopped sandbox",
	Args:  cobra.ExactArgs(1),
	RunE:  runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	name := args[0]
	paths := config.DefaultPaths()

	_, err := config.LoadSandboxMetadata(paths.SandboxesDir, name)
	if err != nil {
		return fmt.Errorf("sandbox not found: %s", name)
	}

	if container.IsRunning(name) {
		logInfo("Sandbox %s is already running", name)
		return nil
	}

	containerName := config.ContainerName(name)
	startCmd := exec.Command("sudo", "machinectl", "start", containerName)
	if err := startCmd.Run(); err != nil {
		return fmt.Errorf("failed to start sandbox: %w", err)
	}

	logSuccess("Started sandbox %s", name)
	return nil
}
