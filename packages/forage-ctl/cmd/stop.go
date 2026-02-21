package cmd

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/audit"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
)

var stopCmd = &cobra.Command{
	Use:   "stop <name>",
	Short: "Stop a running sandbox",
	Args:  cobra.ExactArgs(1),
	RunE:  runStop,
}

var stopTimeout int

func init() {
	stopCmd.Flags().IntVarP(&stopTimeout, "timeout", "t", 30, "Graceful shutdown timeout in seconds (0 for immediate)")
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	name := args[0]

	_, err := loadRunningSandbox(name)
	if err != nil {
		return err
	}

	rt := getRuntime()
	ctx := context.Background()

	var stopErr error
	if stopTimeout > 0 {
		if gs, ok := rt.(runtime.GracefulStopper); ok {
			logInfo("Stopping sandbox %s (timeout: %ds)...", name, stopTimeout)
			stopErr = gs.GracefulStop(ctx, name, time.Duration(stopTimeout)*time.Second)
		} else {
			logInfo("Stopping sandbox %s...", name)
			stopErr = rt.Stop(ctx, name)
		}
	} else {
		logInfo("Stopping sandbox %s...", name)
		stopErr = rt.Stop(ctx, name)
	}

	if stopErr != nil {
		return errors.ContainerFailed("stop", stopErr)
	}

	auditLog := audit.NewLogger(paths().StateDir)
	_ = auditLog.LogEvent(audit.EventStop, name, "")

	logSuccess("Stopped sandbox %s", name)
	return nil
}
