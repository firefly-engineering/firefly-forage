package cmd

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/audit"
	"github.com/firefly-engineering/firefly-forage/packages/forage-ctl/internal/monitor"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor sandbox health in the background",
	Long: `Periodically checks the health of all sandboxes and optionally
restarts unhealthy containers. Runs in the foreground until interrupted.

Can be wrapped in a systemd service for persistent monitoring.`,
	RunE: runMonitor,
}

var (
	monitorInterval    int
	monitorAutoRestart bool
)

func init() {
	monitorCmd.Flags().IntVar(&monitorInterval, "interval", 60, "Health check interval in seconds")
	monitorCmd.Flags().BoolVar(&monitorAutoRestart, "auto-restart", false, "Automatically restart unhealthy containers")
	rootCmd.AddCommand(monitorCmd)
}

func runMonitor(cmd *cobra.Command, args []string) error {
	p := paths()
	rt := getRuntime()
	auditLogger := audit.NewLogger(p.StateDir)

	interval := time.Duration(monitorInterval) * time.Second

	opts := []monitor.Option{
		monitor.WithAuditLogger(auditLogger),
	}
	if monitorAutoRestart {
		opts = append(opts, monitor.WithAutoRestart(true))
	}

	mon := monitor.New(interval, rt, p, opts...)

	logInfo("Starting health monitor (interval: %ds, auto-restart: %v)", monitorInterval, monitorAutoRestart)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err := mon.Run(ctx)
	if err == context.Canceled {
		logInfo("Monitor stopped")
		return nil
	}
	return err
}
