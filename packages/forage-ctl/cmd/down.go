package cmd

import (
	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/sandbox"
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
	p := paths()
	rt := getRuntime()

	logging.Debug("removing sandbox", "name", name)

	metadata, err := config.LoadSandboxMetadata(p.SandboxesDir, name)
	if err != nil {
		return errors.SandboxNotFound(name)
	}

	logInfo("Removing sandbox %s...", name)

	// Use unified cleanup function
	sandbox.Cleanup(metadata, p, sandbox.DefaultCleanupOptions(), rt)

	logSuccess("Removed sandbox %s", name)
	return nil
}
