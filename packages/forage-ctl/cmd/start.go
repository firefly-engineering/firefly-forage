package cmd

import (
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
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
		return errors.SandboxNotFound(name)
	}

	if runtime.IsRunning(name) {
		logInfo("Sandbox %s is already running", name)
		return nil
	}

	if err := runtime.Start(name); err != nil {
		return errors.ContainerFailed("start", err)
	}

	logSuccess("Started sandbox %s", name)
	return nil
}
