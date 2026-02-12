package cmd

import (
	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/app"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/audit"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
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

	_, err := loadSandbox(name)
	if err != nil {
		return err
	}

	if isRunning(name) {
		logInfo("Sandbox %s is already running", name)
		return nil
	}

	if err := app.Default.Start(name); err != nil {
		return errors.ContainerFailed("start", err)
	}

	auditLog := audit.NewLogger(paths().StateDir)
	_ = auditLog.LogEvent(audit.EventStart, name, "")

	logSuccess("Started sandbox %s", name)
	return nil
}
