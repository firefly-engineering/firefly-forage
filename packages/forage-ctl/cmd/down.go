package cmd

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/audit"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/config"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/errors"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/logging"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/runtime"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/sandbox"
)

var downCmd = &cobra.Command{
	Use:   "down <name>",
	Short: "Stop and remove a sandbox",
	Args:  cobra.ExactArgs(1),
	RunE:  runDown,
}

var downTimeout int

func init() {
	downCmd.Flags().IntVarP(&downTimeout, "timeout", "t", 30, "Graceful shutdown timeout in seconds before destroy (0 for immediate)")
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

	// Attempt graceful stop before destroy if timeout > 0
	if downTimeout > 0 {
		if gs, ok := rt.(runtime.GracefulStopper); ok {
			ctx := context.Background()
			running, _ := rt.IsRunning(ctx, name)
			if running {
				logging.Debug("attempting graceful stop before destroy", "timeout", downTimeout)
				_ = gs.GracefulStop(ctx, name, time.Duration(downTimeout)*time.Second)
			}
		}
	}

	// Use unified cleanup function
	sandbox.Cleanup(metadata, p, sandbox.DefaultCleanupOptions(), rt)

	auditLog := audit.NewLogger(p.StateDir)
	_ = auditLog.LogEvent(audit.EventDestroy, name, "")

	logSuccess("Removed sandbox %s", name)
	return nil
}
